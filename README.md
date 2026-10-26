# Pico + Seengreat adapter + 64x32 HUB75 panel

Text on a Waveshare-style 64x32 (1/16 scan) HUB75 panel, driven by an original Raspberry Pi Pico (RP2040) on a Seengreat RGB Matrix Adapter Board V3.9.

- `matrix_fw.c`: firmware that turns the Pico into a USB-serial framebuffer (4096 colours), controlled from Go.
- `go/`: Go library (`go/matrix`), HTTP server (`go/cmd/matrixd`), prayer times (`go/prayer`, `go/prayerclock`), Connect 4 (`go/connect4`), planets (`go/planets`), Pokémon (`go/pokemon`).
- `hub75_test.c`: standalone bit-banged test that shows HELLO! / PICO / V3.9 without a host.
- `main.c` (PIO driver) and `diagnostic.c` are earlier attempts that never worked on this hardware.

## Hardware checklist (each of these cost a debugging session)

- **Pico orientation:** the Pico's micro-USB goes at the same end as the adapter's USB-C port (there is a USB symbol on the adapter's Pico outline). Fitted backwards, the Pico boots and blinks normally but the adapter's 5 V lands on GP14, which damaged an adapter's buffers.
- **Ribbon into the panel's INPUT connector.** The output connector shows nothing.
- **Power the adapter** through its USB-C or DC jack (5 V, several amps) and run its PWR OUT to the panel's VCC/GND. The Pico's USB alone does not power the adapter's level shifters.
- **Pico 2 W is not supported** by the adapter (per Seengreat). Original Pico, Pico W and Pico 2 are.

## Pin map (Seengreat V3.9)

| HUB75 | Pico | HUB75 | Pico |
|---|---|---|---|
| R1, G1, B1 | GP2, GP3, GP4 | A, B, C, D, E | GP8, GP9, GP10, GP11, GP12 |
| R2, G2, B2 | GP5, GP6, GP7 | CLK, LAT, OE | GP13, GP14, GP15 |

V3.8 and earlier boards use a different map; set `PINOUT` to 1 in `hub75_test.c` for those.

## Build and flash

```bash
export PICO_SDK_PATH=/tmp/pico-sdk
export PICO_TOOLCHAIN_PATH=/tmp/pico-tools/arm-gnu-toolchain-14.2.rel1-x86_64-arm-none-eabi
cmake -S . -B build-rp2040 -G Ninja -DPICO_BOARD=pico
cmake --build build-rp2040 --target matrix_fw   # or hub75_test
```

Hold BOOTSEL while plugging in the Pico, then copy `build-rp2040/matrix_fw.uf2` to the `RPI-RP2` drive. The panel shows a dim yellow border until a host sends a frame.

## Controlling it from Go

Your user needs access to the Pico's serial port. Once:

```bash
echo 'SUBSYSTEM=="tty", ATTRS{idVendor}=="2e8a", MODE="0666"' | sudo tee /etc/udev/rules.d/99-pico.rules
sudo udevadm control --reload && sudo udevadm trigger
```

### HTTP server

```bash
cd go && go run ./cmd/matrixd            # http://127.0.0.1:8080 has a control page
go run ./cmd/matrixd -addr :8080         # reachable from other machines on the network (no auth!)
```

```bash
curl -d 'Hello world' -H 'Content-Type: text/plain' localhost:8080/api/text
curl -d '{"text":"HI\nTHERE","color":"#ff8000","scroll":"never"}' localhost:8080/api/text
curl -d 'I :heart: GO 🔥' -H 'Content-Type: text/plain' localhost:8080/api/text
curl localhost:8080/api/emoji            # list built-in emoji and their :shortcodes:
curl -d '{"brightness":60}' localhost:8080/api/brightness
curl --data-binary @picture.png localhost:8080/api/image
curl -X POST localhost:8080/api/clear
curl localhost:8080/api/state
```

`/api/text` fields: `text` (up to 4 lines, `\n`-separated), `color`/`background` (`#rrggbb` or a name like `red`), `align` (`left`/`center`/`right`), `scroll` (`auto` scrolls only when too wide, `always`, `never`), `speed` (px/s).

Emoji: text can contain emoji characters (❤️ 🔥 ⭐ 😎 👾 😂 💩 🪐 🔭 …) or `:shortcodes:` (`:heart:`, `:fire:`, `:invader:`, `:joy:`/`:lol:`, `:poop:`, `:planet:`, `:telescope:` …). They're 7×7 colour sprites defined in `go/matrix/emoji.go`; add your own there. Unsupported emoji show as `?`.

**State is saved.** After every change, `matrixd` writes what the panel shows (text, prayer clock, planet, rotation, game or image) and the brightness to `~/.config/matrixd/state.json`, and restores it on the next start. Passing `-text`, `-prayer` or `-rotate` overrides the saved state for that start; `-state other.json` uses a different file and `-state ''` turns saving off.

### Saved scenes

Save whatever the panel is showing (text with its colours and scrolling, a planet, a rotation, an image…) under a name, and show it again later from the control page's **Saved scenes** section. Scenes are kept in the state file, so they survive restarts. Showing a scene leaves the brightness as it is. Saving under an existing name replaces that scene. Connect 4 games can't be saved as scenes.

```bash
curl -d '{"name":"Welcome"}' localhost:8080/api/scenes        # save what's showing now
curl localhost:8080/api/scenes                                 # list them
curl -X POST localhost:8080/api/scenes/Welcome/show            # show one again
curl -X DELETE localhost:8080/api/scenes/Welcome
```

### Prayer times

Shows the current time with a time-of-day emoji (🌙 night, 🌅 Fajr to sunrise, ☀ day, 🌤 after Asr, 🌇 after Maghrib), then the next prayer, its time and a countdown. The calculation is `go/prayer` (imported from [this gist](https://gist.github.com/rbrick/e971ac3fbe69240da74aeb6cd3e68681)); the display is `go/prayerclock`.

```bash
./bin/matrixd -prayer -lat 41.8781 -long -87.6298 -method ISNA     # start in prayer mode
curl -d '{"lat":41.8781,"long":-87.6298,"method":"ISNA","clock24":false}' localhost:8080/api/prayer
```

Methods: `ISNA`, `MWL`, `EGAS`, `Karachi`, `UAE`, `Kuwait`. The control page groups controls into tabs. Set latitude/longitude once in the **Location** section of the Weather tab (or choose **Use my location**); weather and prayer times share those coordinates, which are saved in this browser.

### Weather scene

A full-panel animated landscape: Open-Meteo supplies only current weather; the prayer clock's solar calculations determine local sunrise, sunset, and the Sun's real-time height and east-to-west progress from your coordinates. Daylight and warm twilight change gradually; a swirling Milky Way, golden star halos and a chunky eight-step moon phase travel through the night, with moving layered clouds, rain and lightning for current conditions. Wavy cobalt, turquoise and ochre sky strokes, textured hills and a dark foreground cypress take inspiration from Van Gogh's *The Starry Night*.

```bash
curl -d '{"lat":41.8781,"long":-87.6298,"refresh":600}' localhost:8080/api/weather
```

The control page uses the shared location from the Weather tab. Its debug controls preview sunrise and sunset (each loops through 2½ hours of twilight in 2½ minutes), night (dusk through sunrise in 2½ minutes, with the moon setting as the sun rises), rain or thunderstorms. Time and weather previews can be combined. The optional `"preview":"sunset"` and `"weatherPreview":"storm"` fields also work with `/api/weather`; set them to `""` to return to live conditions. Weather is refreshed every 10 minutes by default (minimum one minute); the last conditions remain visible during a temporary API failure, while solar timing continues locally.

### Planets

Rotating, sunlit 3D planets over a twinkling star field: the Sun, Mercury, Venus, Earth (with drifting clouds), the Moon, Mars, Jupiter (Great Red Spot), Saturn (rings in front of and behind the planet), Uranus (on its side), Neptune and Pluto (with its heart). Pick one, or `tour` to cycle through them all.

```bash
curl -d '{"body":"saturn"}' localhost:8080/api/planet
curl -d '{"body":"tour","dwell":15,"speed":1.5,"label":true}' localhost:8080/api/planet
```

### Solar system

Every planet (plus Pluto) where it really is around the Sun right now, looking down on the solar system from 29° above its plane. Positions come from JPL's orbital elements for the planets (Standish, valid 1800–2050) and Kepler's equation, in `go/orrery`; the tests check them against published oppositions and conjunctions. **Directions are exact, distances are not:** at true scale the inner four planets would sit inside the Sun's pixels, so each orbit gets its own ring and distances in between are interpolated (eccentric orbits like Mercury's and Pluto's keep their shape, and Pluto's tilt shows). Orbits are faint dots in each planet's colour; Jupiter and Saturn are bigger, and Saturn has its rings.

```bash
curl -X POST localhost:8080/api/solarsystem                       # live, naming each planet in turn
curl -d '{"speed":30,"label":false}' localhost:8080/api/solarsystem   # time-lapse: 30 days per second, with the date
```

### Minecraft server

Pings a Minecraft Java Edition server (the same Server List Ping the multiplayer screen uses) and shows its address along the top (scrolling if it's long), a grass block or the server's own icon, the latency, and how many players are online. It pings again every 5 seconds while on screen (`"refresh"` sets the interval, minimum 2), and shows OFFLINE in red if the server can't be reached. Addresses work like in the game: `host`, `host:port`, or a name with a `_minecraft._tcp` SRV record. The code is `go/minecraft`, based on [this gist](https://gist.github.com/rbrick/f390b19d0287a4ae01d56b6ce57aa764).

```bash
curl -d '{"address":"mc.hypixel.net"}' localhost:8080/api/minecraft
curl -d '{"address":"play.cubecraft.net","icon":"server","refresh":2}' localhost:8080/api/minecraft
```

The reply's `minecraftStatus` has the result: players, max, latency, version and MOTD (also shown on the control page).

### Starfront roofs

Shows whether the Starfront observatory's roll-off roofs are open, from its Alpaca API (`https://alpaca-api.tx.starfront.space/api/v1/roof/state`), checked every 15 seconds while on screen (`"refresh"`, minimum 5). The headline is a consensus: OPEN or CLOSED when most buildings agree, and MIXED in amber on a tie. That way one roof misreporting can't flip it: if building 1 says open and the rest say closed, the panel reads CLOSED, "1 OF 14 OPEN". A telescope sits beside the headline, and a row of squares along the bottom shows every building (green open, red closed). A building whose last reading is over 30 minutes old is grey and doesn't vote. Set `"building"` to show that building's own state instead, with its square underlined. If the API can't be reached, the panel keeps the last reading and says NO CONNECTION. The code is `go/starfront`.

```bash
curl -d '{}' localhost:8080/api/starfront
curl -d '{"building":5,"refresh":30}' localhost:8080/api/starfront
curl localhost:8080/api/starfront/roofs   # every building's latest reading
```

The reply's `starfrontStatus.roofs` has each building's reading (also summarised on the control page).

### Neo Tokyo

**$BYTES price**: the price of Neo Tokyo's $BYTES token from `https://api.bytes.onl/v1/api/price`, checked every 30 seconds while on screen (`"refresh"`, minimum 10). The price is in big type, and turns green or red when it moves up or down. Under it are the price in ETH and the market cap. If the API can't be reached, the last price stays up with NO CONNECTION underneath.

```bash
curl -d '{}' localhost:8080/api/bytes
curl -d '{"refresh":60}' localhost:8080/api/bytes
```

**Citizens**: a Neo Tokyo Citizen rendered by `https://citgen.rcw.io/s{season}/{w}x{h}/{id}`. matrixd downloads it at 64×64 (cached in `~/.cache/matrixd/citizens`) and shows it on the left with CITIZEN #2648 SEASON 1 beside it. By default (`"zoom": true`) it shows the face at full detail; `"zoom": false` shrinks the whole picture instead, and `"label": false` centres the picture with no text. Leave out `"id"` for a different random Citizen every `"dwell"` seconds (default 20), from `"season"` 1 or 2, or either when the season is left out. IDs have gaps, so a random pick tries again when an ID doesn't exist. A new Citizen takes the server about 3 seconds to render.

```bash
curl -d '{"season":1,"id":2648}' localhost:8080/api/citizen
curl -d '{"season":2,"zoom":false}' localhost:8080/api/citizen   # random Season 2 Citizens
curl -d '{}' localhost:8080/api/citizen                          # random from either season
```

The code is `go/neotokyo`.

### Moon phase

Today's Moon, lit as it really is, with the phase name, how much is lit, and the date of the next full or new moon. The Sun's position comes from the prayer-time code (`go/prayer`); the Moon's from the main terms of the lunar series in Meeus' *Astronomical Algorithms*, good to a few minutes for the moment of each phase. The calculation and scene are `go/moonphase`; the Moon is drawn with the planets' Moon texture.

```bash
curl -X POST localhost:8080/api/moon
curl -d '{"south":true}' localhost:8080/api/moon     # southern hemisphere: waxing Moon lit on the left
```

### Rotation

Takes turns between views, fading from one to the next. Each view picks up where it left off, so a planet `tour` shows the next planet on each turn.

```bash
# prayer times for 3 minutes, then a planet for 20 s, then the moon for 20 s
./bin/matrixd -rotate -lat 41.8781 -long -87.6298

curl -d '{"items":[
  {"mode":"prayer","prayer":{"lat":41.8781,"long":-87.6298,"method":"ISNA"},"seconds":180},
  {"mode":"planet","planet":{"body":"tour","dwell":20},"seconds":20},
  {"mode":"moon","moon":{},"seconds":20},
  {"mode":"pokemon","pokemon":{"pokemon":"random","dwell":20},"seconds":20},
  {"mode":"text","text":{"text":"HI :heart:"},"seconds":10}
]}' localhost:8080/api/rotation
```

Each item is a `mode` (`prayer`, `planet`, `moon`, `solarsystem`, `minecraft`, `starfront`, `bytes`, `citizen`, `pokemon` or `text`), that mode's settings (the same as its own endpoint), and `seconds`. On the control page, tick the views in the Rotation section; each uses the settings in its own section.

### Pokémon

Box sprites (the icons from the PC storage boxes) for all 905 Pokémon from [PokéSprite](https://github.com/msikma/pokesprite), with the name and Pokédex number, a little hop like in the games, and shiny variants. Sprites are downloaded on first use and cached in `~/.cache/matrixd/pokesprite`; ones too big for the panel are scaled down, the rest are shown pixel-for-pixel.

```bash
curl -d '{"pokemon":"pikachu"}' localhost:8080/api/pokemon
curl -d '{"pokemon":"130","shiny":true}' localhost:8080/api/pokemon
curl -d '{"pokemon":"random","dwell":15}' localhost:8080/api/pokemon     # a new one every 15s
```

Sprites also work as still images inside any text, by shortcode: `:pikachu:`, `:mr-mime:`, `:shiny-gyarados:`. The line grows to the sprite's height with the text centred beside it, so `curl -d 'I :heart: :pikachu:' -H 'Content-Type: text/plain' localhost:8080/api/text` shows a heart and Pikachu side by side. Numbers aren't shortcodes (so `12:30:45` stays a time) and built-in emoji win any name clash.

Poké Balls work the same way: `:pokeball:`, `:greatball:`, `:ultraball:`, `:masterball:`, `:hisuian-pokeball:` and the rest of PokéSprite's 38 balls (the shortcode is the ball's name ending in `ball`, so `:moonball:` is the Moon Ball while `:moon:` stays the moon emoji). The control page has a row of ball buttons under the emoji; `GET /api/pokemon/balls` lists them.

Pokémon sprites are © Nintendo / Game Freak / Creatures; PokéSprite's data and code are MIT licensed.

### Connect 4

Play on the panel from the control page (click a column, or ← → and Enter/Space/↓), against the computer (easy/normal/hard) or two players taking turns. The board is on the right; the left shows whose turn it is and the score. Games are saved, so a restart picks up mid-game.

#### Bluetooth and USB controllers

`matrixd` automatically discovers Linux game controllers, including Nintendo Switch Pro/Joy-Con and Xbox controllers, when they connect or reconnect. Press **Start/Menu/+** or **Home/Guide** to return to the last controller menu (press it again from a menu for the root menu), then choose with the D-pad and any face button. The Games submenu includes **Connect 4** and **Block Drop** (a falling-block puzzle: Left/Right move, Down drops, Up/A/B/X/Y rotates). Its **Displays** submenu lets you configure and switch directly between Planets, Solar System, Moon Phase, Pokémon, Starfront, $BYTES, Citizens, the welcome display, or any scene saved from the website. Each display opens a controller-friendly options menu (for example planet, time-lapse speed, hemisphere, shiny Pokémon, building, refresh rate, or season). Weather and Prayer Clock appear only after a location has been configured in one of those displays, and offer preview/method choices. Menus remember their previous selection, long items scroll automatically, and the controller's B button goes back (with the correct Nintendo/Xbox button layout). You can also resume the previous display or turn the panel off. USB controllers work immediately.

Pair a Bluetooth controller once on the Pi (put the controller in pairing mode first):

```bash
sudo apt install bluez
sudo systemctl enable --now bluetooth
bluetoothctl
# At the bluetoothctl prompt:
power on
agent on
default-agent
scan on
# After your controller appears, replace XX:... with its address:
pair XX:XX:XX:XX:XX:XX
trust XX:XX:XX:XX:XX:XX
connect XX:XX:XX:XX:XX:XX
quit
```

A trusted controller reconnects automatically afterward. Hold the small Sync button on a Switch controller or the Pair button on an Xbox controller to enter pairing mode. Controller support is enabled by default; start `matrixd` with `-gamepad=false` to disable it.

```bash
curl -d '{"vsComputer":true,"difficulty":"hard"}' localhost:8080/api/connect4/new
curl -d '{"column":3}' localhost:8080/api/connect4/move
curl localhost:8080/api/connect4
```

### Library

```go
import "dotmatrix/matrix"

// High level: keeps content up, scrolls, reconnects after unplugging.
p := matrix.NewPlayer("") // "" finds the Pico automatically
p.ShowText(matrix.Text{Text: "Hello from Go", Color: color.RGBA{0, 200, 255, 255}})

// Low level: draw anything (Canvas is a draw.Image) and push it.
d, err := matrix.Open("")
c := matrix.NewCanvas()
c.DrawText(2, 2, "HI", color.White)
d.Show(c)
```

Only one program can hold the serial port at a time, so stop `matrixd` before using the library directly.

### Driving the panel directly from a Raspberry Pi

The Seengreat adapter's Raspberry Pi header uses the standard `rpi-rgb-led-matrix` pinout. On 64-bit Raspberry Pi OS/Debian, install the GPIO driver and this repository's small bridge:

```bash
sudo apt install git build-essential
sudo git clone --depth 1 https://github.com/hzeller/rpi-rgb-led-matrix /opt/rpi-rgb-led-matrix
make -C pi
sudo make -C pi install
cd go && go build -o matrixd ./cmd/matrixd
sudo ./matrixd -port pi -addr :8080
```

`matrix-pi` defaults to one 64x32 FM6126A panel, the `regular` GPIO mapping and GPIO slowdown 2 for a Pi 3B+. Options accepted by `rpi-rgb-led-matrix` can be passed to the bridge directly for hardware diagnosis. The HTTP API is unchanged, so web, desktop and mobile clients do not need hardware-specific code.

Do not run other software that uses these GPIO pins at the same time. For reliable refresh, disable onboard audio and keep the Pi out of heavy thermal throttling as recommended by `rpi-rgb-led-matrix`.

### Wire protocol

Single-byte commands over the USB serial port: `F` + 4096 bytes (64×32 little-endian `0x0RGB` pixels) shows a frame, `B` + 1 byte sets brightness, both answer `K\n`; `P` answers `PICO-MATRIX 1 64 32\n`.

(The SDK and toolchain currently live in `/tmp` and will vanish on reboot; move them somewhere permanent if you keep working on this.)

