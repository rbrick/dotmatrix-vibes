#include <stdio.h>
#include <string.h>
#include "pico/stdlib.h"
#include "hardware/gpio.h"
#include "hardware/pio.h"
#include "hub75.pio.h"

// This program assumes a standard 64x32, 1/16-scan HUB75 panel.
#define WIDTH 64
#define HEIGHT 32

// Seengreat RGB Matrix Adapter Board v3.9 wiring. The photo shows "RGB
// Matrix 3.9", whose pins are arranged consecutively specifically for PIO.
#define RGB_BASE_PIN 2   // GP2..GP7 = R1,G1,B1,R2,G2,B2
#define ROW_BASE_PIN 8   // GP8..GP11 = A,B,C,D (GP12/E unused by this 16S panel)
#define CLOCK_PIN 13     // GP13 = CLK
#define LATCH_PIN 14     // GP14 = LAT; GP15 = OE

// Change this text, rebuild, and copy the new UF2 to the Pico.
#define MESSAGE "HELLO PICO"

// Framebuffer pixels are 0x00BBGGRR (the order expected by the PIO program).
static uint32_t pixels[HEIGHT][WIDTH];

static inline uint32_t rgb(uint8_t r, uint8_t g, uint8_t b) {
    return r | ((uint32_t)g << 8) | ((uint32_t)b << 16);
}

// Tiny readable 5x7 font. Each byte is one vertical column, least-significant bit
// at the top. Add another entry here if you need a character not listed.
typedef struct { char character; uint8_t column[5]; } glyph_t;
static const glyph_t font[] = {
    {' ', {0,0,0,0,0}}, {'!', {0,0,0x5f,0,0}}, {'-', {8,8,8,8,8}},
    {'0', {0x3e,0x51,0x49,0x45,0x3e}}, {'1', {0,0x42,0x7f,0x40,0}},
    {'2', {0x42,0x61,0x51,0x49,0x46}}, {'3', {0x21,0x41,0x45,0x4b,0x31}},
    {'4', {0x18,0x14,0x12,0x7f,0x10}}, {'5', {0x27,0x45,0x45,0x45,0x39}},
    {'6', {0x3c,0x4a,0x49,0x49,0x30}}, {'7', {1,0x71,9,5,3}},
    {'8', {0x36,0x49,0x49,0x49,0x36}}, {'9', {6,0x49,0x49,0x29,0x1e}},
    {'A', {0x7e,0x11,0x11,0x11,0x7e}}, {'B', {0x7f,0x49,0x49,0x49,0x36}},
    {'C', {0x3e,0x41,0x41,0x41,0x22}}, {'D', {0x7f,0x41,0x41,0x22,0x1c}},
    {'E', {0x7f,0x49,0x49,0x49,0x41}}, {'F', {0x7f,9,9,9,1}},
    {'G', {0x3e,0x41,0x49,0x49,0x7a}}, {'H', {0x7f,8,8,8,0x7f}},
    {'I', {0,0x41,0x7f,0x41,0}}, {'J', {0x20,0x40,0x41,0x3f,1}},
    {'K', {0x7f,8,0x14,0x22,0x41}}, {'L', {0x7f,0x40,0x40,0x40,0x40}},
    {'M', {0x7f,2,0x0c,2,0x7f}}, {'N', {0x7f,4,8,0x10,0x7f}},
    {'O', {0x3e,0x41,0x41,0x41,0x3e}}, {'P', {0x7f,9,9,9,6}},
    {'Q', {0x3e,0x41,0x51,0x21,0x5e}}, {'R', {0x7f,9,0x19,0x29,0x46}},
    {'S', {0x46,0x49,0x49,0x49,0x31}}, {'T', {1,1,0x7f,1,1}},
    {'U', {0x3f,0x40,0x40,0x40,0x3f}}, {'V', {0x1f,0x20,0x40,0x20,0x1f}},
    {'W', {0x3f,0x40,0x38,0x40,0x3f}}, {'X', {0x63,0x14,8,0x14,0x63}},
    {'Y', {7,8,0x70,8,7}}, {'Z', {0x61,0x51,0x49,0x45,0x43}}
};

static const uint8_t *find_glyph(char c) {
    for (size_t i = 0; i < sizeof(font) / sizeof(font[0]); ++i)
        if (font[i].character == c) return font[i].column;
    return font[0].column; // Unknown characters become spaces.
}

static void draw_text(const char *text, int x, int y, uint32_t colour) {
    while (*text) {
        const uint8_t *glyph = find_glyph(*text++);
        for (int column = 0; column < 5; ++column)
            for (int row = 0; row < 7; ++row)
                if ((glyph[column] >> row) & 1) {
                    int px = x + column, py = y + row;
                    if (px >= 0 && px < WIDTH && py >= 0 && py < HEIGHT)
                        pixels[py][px] = colour;
                }
        x += 6; // Five pixels plus a one-pixel gap.
    }
}

int main(void) {
    stdio_init_all();

    // Turn on the Pico's onboard green LED as a simple heartbeat: if this is
    // solid, this exact firmware booted and reached main().
#ifdef PICO_DEFAULT_LED_PIN
    gpio_init(PICO_DEFAULT_LED_PIN);
    gpio_set_dir(PICO_DEFAULT_LED_PIN, GPIO_OUT);
    gpio_put(PICO_DEFAULT_LED_PIN, 1);
#endif

    sleep_ms(1000);
    printf("Starting 64x32 HUB75 display\n");

    // Diagnostic pattern: four short horizontal bars. This independently tests
    // all colour channels and both halves while keeping power consumption low.
    memset(pixels, 0, sizeof(pixels));
    for (int y = 2; y < 6; ++y)
        for (int x = 4; x < WIDTH - 4; ++x) pixels[y][x] = rgb(255, 0, 0);
    for (int y = 10; y < 14; ++y)
        for (int x = 4; x < WIDTH - 4; ++x) pixels[y][x] = rgb(0, 255, 0);
    for (int y = 18; y < 22; ++y)
        for (int x = 4; x < WIDTH - 4; ++x) pixels[y][x] = rgb(0, 0, 255);
    for (int y = 26; y < 30; ++y)
        for (int x = 4; x < WIDTH - 4; ++x) pixels[y][x] = rgb(255, 255, 255);

    // Original PIO implementation: this is the version that previously caused
    // visible blue flashes. Restoring it lets us reproduce that useful signal.
    PIO pio = pio0;
    uint data_offset = pio_add_program(pio, &hub75_data_rgb888_program);
    uint row_offset = pio_add_program(pio, &hub75_row_program);
    hub75_data_rgb888_program_init(pio, 0, data_offset, RGB_BASE_PIN, CLOCK_PIN);
    hub75_row_program_init(pio, 1, row_offset, ROW_BASE_PIN, LATCH_PIN);

    while (true) {
        for (int row = 0; row < 16; ++row) {
            for (int bit = 0; bit < 8; ++bit) {
                hub75_data_rgb888_set_shift(pio, 0, data_offset, bit);
                for (int x = 0; x < WIDTH; ++x) {
                    pio_sm_put_blocking(pio, 0, pixels[row][x]);
                    pio_sm_put_blocking(pio, 0, pixels[row + 16][x]);
                }
                pio_sm_put_blocking(pio, 0, 0);
                pio_sm_put_blocking(pio, 0, 0);
                hub75_wait_tx_stall(pio, 0);
                hub75_wait_tx_stall(pio, 1);
                pio_sm_put_blocking(pio, 1,
                    (uint32_t)row | ((20u << bit) << 4));
            }
        }
    }
}
