package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

type bluetoothDevice struct {
	Address   string `json:"address"`
	Name      string `json:"name"`
	Icon      string `json:"icon,omitempty"`
	Paired    bool   `json:"paired"`
	Trusted   bool   `json:"trusted"`
	Connected bool   `json:"connected"`
}

var bluetoothAddress = regexp.MustCompile(`(?i)^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)

type bluetoothAPI struct {
	mu       sync.Mutex
	scanning bool
}

func handleBluetooth(mux *http.ServeMux) {
	api := &bluetoothAPI{}
	mux.HandleFunc("GET /api/bluetooth/controllers", func(w http.ResponseWriter, r *http.Request) {
		devices, err := listBluetoothDevices()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		api.mu.Lock()
		scanning := api.scanning
		api.mu.Unlock()
		writeJSON(w, struct {
			Scanning bool              `json:"scanning"`
			Devices  []bluetoothDevice `json:"devices"`
		}{scanning, devices})
	})
	mux.HandleFunc("POST /api/bluetooth/scan", func(w http.ResponseWriter, r *http.Request) {
		if err := api.startScan(); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, map[string]bool{"scanning": true})
	})
	mux.HandleFunc("POST /api/bluetooth/pair", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Address string `json:"address"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req) != nil || !bluetoothAddress.MatchString(req.Address) {
			http.Error(w, "invalid Bluetooth address", http.StatusBadRequest)
			return
		}
		address := strings.ToUpper(req.Address)
		if out, err := exec.Command("bluetoothctl", "--timeout", "30", "--agent", "NoInputNoOutput", "pair", address).CombinedOutput(); err != nil {
			http.Error(w, commandError("pairing", out, err).Error(), http.StatusBadGateway)
			return
		}
		if out, err := exec.Command("bluetoothctl", "trust", address).CombinedOutput(); err != nil {
			http.Error(w, commandError("trusting", out, err).Error(), http.StatusBadGateway)
			return
		}
		// Some controllers connect during pairing; others need an explicit connect.
		if out, err := exec.Command("bluetoothctl", "connect", address).CombinedOutput(); err != nil {
			http.Error(w, commandError("connecting", out, err).Error(), http.StatusBadGateway)
			return
		}
		devices, _ := listBluetoothDevices()
		writeJSON(w, devices)
	})
	mux.HandleFunc("POST /api/bluetooth/connect", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Address string `json:"address"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req) != nil || !bluetoothAddress.MatchString(req.Address) {
			http.Error(w, "invalid Bluetooth address", http.StatusBadRequest)
			return
		}
		if out, err := exec.Command("bluetoothctl", "connect", strings.ToUpper(req.Address)).CombinedOutput(); err != nil {
			http.Error(w, commandError("connecting", out, err).Error(), http.StatusBadGateway)
			return
		}
		devices, _ := listBluetoothDevices()
		writeJSON(w, devices)
	})
}

func (b *bluetoothAPI) startScan() error {
	b.mu.Lock()
	if b.scanning {
		b.mu.Unlock()
		return nil
	}
	b.mu.Unlock()
	// Raspberry Pi OS may boot with Bluetooth soft-blocked. matrixd's appliance
	// service runs as root, so make the adapter ready when the user asks to scan.
	_ = exec.Command("rfkill", "unblock", "bluetooth").Run()
	if out, err := exec.Command("bluetoothctl", "power", "on").CombinedOutput(); err != nil {
		return commandError("enabling Bluetooth", out, err)
	}
	b.mu.Lock()
	b.scanning = true
	b.mu.Unlock()
	go func() {
		defer func() { b.mu.Lock(); b.scanning = false; b.mu.Unlock() }()
		// Discovery remains active while bluetoothctl is alive.
		_ = exec.Command("bluetoothctl", "--timeout", "30", "scan", "on").Run()
	}()
	return nil
}

func listBluetoothDevices() ([]bluetoothDevice, error) {
	out, err := exec.Command("bluetoothctl", "devices").Output()
	if err != nil {
		return nil, fmt.Errorf("Bluetooth is unavailable: %w", err)
	}
	devices := make([]bluetoothDevice, 0)
	s := bufio.NewScanner(strings.NewReader(string(out)))
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 3 || fields[0] != "Device" || !bluetoothAddress.MatchString(fields[1]) {
			continue
		}
		d := bluetoothDevice{Address: strings.ToUpper(fields[1]), Name: strings.Join(fields[2:], " ")}
		if info, e := exec.Command("bluetoothctl", "info", d.Address).Output(); e == nil {
			parseBluetoothInfo(&d, string(info))
		}
		// Keep paired devices and likely input devices; hide TVs, phones, etc.
		if d.Paired || d.Icon == "input-gaming" || strings.Contains(strings.ToLower(d.Name), "controller") || strings.Contains(strings.ToLower(d.Name), "joy-con") || strings.Contains(strings.ToLower(d.Name), "xbox") {
			devices = append(devices, d)
		}
	}
	return devices, s.Err()
}

func parseBluetoothInfo(d *bluetoothDevice, info string) {
	for _, line := range strings.Split(info, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok {
			continue
		}
		switch key {
		case "Name", "Alias":
			if value != "" {
				d.Name = value
			}
		case "Icon":
			d.Icon = value
		case "Paired":
			d.Paired = value == "yes"
		case "Trusted":
			d.Trusted = value == "yes"
		case "Connected":
			d.Connected = value == "yes"
		}
	}
}

func commandError(action string, out []byte, err error) error {
	message := strings.TrimSpace(string(out))
	if message == "" {
		message = err.Error()
	}
	return errors.New(action + " controller: " + message)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}
