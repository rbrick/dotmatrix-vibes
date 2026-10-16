// USB-serial framebuffer for a 64x32 HUB75 panel on a Seengreat RGB Matrix
// Adapter Board V3.9 with an original Raspberry Pi Pico (RP2040).
//
// The host (see go/) renders everything and streams whole frames; this
// firmware only displays them. Core 1 refreshes the panel continuously with
// 4-bit binary code modulation (4096 colours); core 0 handles USB.
//
// Protocol (host -> Pico), each command is one byte followed by its payload:
//   'F' + 64*32 little-endian uint16 pixels (0x0RGB, 4 bits per channel,
//         row-major from the top-left) -> replies "K\n" once queued
//   'B' + 1 byte brightness (0-255)     -> replies "K\n"
//   'P'                                 -> replies "PICO-MATRIX 1 64 32\n"
// Unknown bytes are ignored, so the host can resynchronise by sending 'P'.

#include <string.h>
#include <stdio.h>
#include "pico/stdlib.h"
#include "pico/multicore.h"
#include "hardware/gpio.h"

#define WIDTH 64
#define HEIGHT 32
#define SCAN_ROWS (HEIGHT / 2)
#define PLANES 4
#define PLANE0_CYCLES 300  // lit-slot length of the least significant bit plane

// Seengreat V3.9 pin map.
#define PIN_R1 2
#define PIN_G1 3
#define PIN_B1 4
#define PIN_R2 5
#define PIN_G2 6
#define PIN_B2 7
#define PIN_A 8  // A..E are GP8..GP12
#define PIN_CLK 13
#define PIN_LAT 14
#define PIN_OE 15

#define RGB_MASK (0x3fu << PIN_R1)
#define ADDR_MASK (0x1fu << PIN_A)
#define ALL_MASK (RGB_MASK | ADDR_MASK | (1u << PIN_CLK) | (1u << PIN_LAT) | (1u << PIN_OE))

static uint16_t frames[2][HEIGHT][WIDTH];
static volatile uint front;              // buffer core 1 is displaying
static volatile bool swap_pending;       // core 0 filled the back buffer
static volatile uint8_t brightness = 128;

static inline void tiny_delay(void) { busy_wait_at_least_cycles(8); }

static void __not_in_flash_func(refresh_forever)(void) {
    while (true) {
        if (swap_pending) {
            front ^= 1;
            __dmb();
            swap_pending = false;
        }
        const uint16_t (*fb)[WIDTH] = frames[front];
        for (uint row = 0; row < SCAN_ROWS; ++row) {
            for (uint plane = 0; plane < PLANES; ++plane) {
                const uint16_t *top = fb[row], *bottom = fb[row + SCAN_ROWS];
                for (uint x = 0; x < WIDTH; ++x) {
                    uint t = top[x], b = bottom[x];
                    // 0x0RGB: red bits 8-11, green 4-7, blue 0-3.
                    uint32_t bits = ((t >> (8 + plane)) & 1) | ((t >> (4 + plane)) & 1) << 1 |
                                    ((t >> plane) & 1) << 2 | ((b >> (8 + plane)) & 1) << 3 |
                                    ((b >> (4 + plane)) & 1) << 4 | ((b >> plane) & 1) << 5;
                    gpio_put_masked(RGB_MASK, bits << PIN_R1);
                    tiny_delay();
                    gpio_put(PIN_CLK, 1);
                    tiny_delay();
                    gpio_put(PIN_CLK, 0);
                }
                gpio_put(PIN_OE, 1);
                gpio_put_masked(ADDR_MASK, row << PIN_A);
                tiny_delay();
                gpio_put(PIN_LAT, 1);
                tiny_delay();
                gpio_put(PIN_LAT, 0);

                uint32_t slot = PLANE0_CYCLES << plane;
                uint32_t lit = slot * brightness / 255;
                if (lit) {
                    gpio_put(PIN_OE, 0);
                    busy_wait_at_least_cycles(lit);
                    gpio_put(PIN_OE, 1);
                }
                busy_wait_at_least_cycles(slot - lit);
            }
        }
    }
}

// FM6126A driver chips need configuring before they show anything; ordinary
// shift-register panels ignore this.
static void fm6126a_init(void) {
    const uint16_t regs[2] = {0x7fff, 0x0040};
    const int latch_cols[2] = {12, 13};
    for (int r = 0; r < 2; ++r) {
        for (int x = 0; x < WIDTH; ++x) {
            gpio_put_masked(RGB_MASK, (regs[r] >> (x % 16)) & 1 ? RGB_MASK : 0);
            gpio_put(PIN_LAT, x >= WIDTH - latch_cols[r]);
            tiny_delay();
            gpio_put(PIN_CLK, 1);
            tiny_delay();
            gpio_put(PIN_CLK, 0);
        }
        gpio_put(PIN_LAT, 0);
    }
}

// Reads exactly len bytes, or returns false if the host stalls mid-command.
static bool read_exact(uint8_t *buf, int len) {
    absolute_time_t deadline = make_timeout_time_ms(500);
    while (len > 0) {
        int n = stdio_get_until((char *)buf, len, deadline);
        if (n <= 0) return false;
        buf += n;
        len -= n;
    }
    return true;
}

static void reply(const char *s) {
    fputs(s, stdout);
    fflush(stdout);
}

// Until the host sends a frame: a dim border so a working panel is obvious.
static void draw_idle_frame(uint16_t (*fb)[WIDTH]) {
    memset(fb, 0, sizeof(frames[0]));
    for (int x = 0; x < WIDTH; ++x) fb[0][x] = fb[HEIGHT - 1][x] = 0x330;
    for (int y = 0; y < HEIGHT; ++y) fb[y][0] = fb[y][WIDTH - 1] = 0x330;
}

int main(void) {
    stdio_init_all();
#ifdef PICO_DEFAULT_LED_PIN  // W boards wire the LED to the wireless chip instead
    gpio_init(PICO_DEFAULT_LED_PIN);
    gpio_set_dir(PICO_DEFAULT_LED_PIN, GPIO_OUT);
#endif

    gpio_init_mask(ALL_MASK);
    gpio_set_dir_out_masked(ALL_MASK);
    gpio_put_masked(ALL_MASK, 1u << PIN_OE);
    fm6126a_init();

    draw_idle_frame(frames[0]);
    multicore_launch_core1(refresh_forever);

    while (true) {
        int c = stdio_getchar_timeout_us(100000);
#ifdef PICO_DEFAULT_LED_PIN
        gpio_put(PICO_DEFAULT_LED_PIN, stdio_usb_connected());
#endif
        if (c == 'F') {
            while (swap_pending) tight_loop_contents();  // back buffer still queued
            uint16_t (*back)[WIDTH] = frames[front ^ 1];
            // The RP2040 is little-endian, matching the wire format.
            if (read_exact((uint8_t *)back, sizeof(frames[0]))) {
                __dmb();
                swap_pending = true;
                reply("K\n");
            } else {
                reply("E timeout\n");
            }
        } else if (c == 'B') {
            uint8_t b;
            if (read_exact(&b, 1)) {
                brightness = b;
                reply("K\n");
            }
        } else if (c == 'P') {
            reply("PICO-MATRIX 1 64 32\n");
        }
    }
}
