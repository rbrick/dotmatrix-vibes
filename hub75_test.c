// Minimal bit-banged HUB75 text test for an original Raspberry Pi Pico (RP2040)
// on a Seengreat RGB Matrix Adapter Board driving a 64x32, 1/16-scan panel.
//
// No PIO or DMA: every signal is toggled directly so there is as little as
// possible to go wrong. Seengreat changed the Pico pinout at board V3.9; set
// PINOUT below to match the revision printed on the adapter.
//
// It also sends the FM6126A configuration sequence before each attempt. Many
// Waveshare panels use FM6126A drivers and stay completely dark without it;
// on ordinary shift-register panels the sequence is harmless.

#include <stdio.h>
#include <string.h>
#include "pico/stdlib.h"
#include "hardware/gpio.h"

#define WIDTH 64
#define HEIGHT 32
#define SCAN_ROWS (HEIGHT / 2)
#define OE_ON_US 40  // Per-row lit time; raise for brightness, lower for less current.

typedef struct {
    const char *name;
    uint r1, g1, b1, r2, g2, b2;
    uint a, b, c, d, e;
    uint clk, lat, oe;
} pinout_t;

// From the Seengreat wiki (RGB Matrix Adapter Board, tables 2-2 / 2-3).
#define PINOUT 0  // 0 = V3.9 and later, 1 = V3.8 and earlier
static const pinout_t pinouts[] = {
    {"V3.9", 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
    {"V3.8", 2, 3, 4, 5, 8, 9, 10, 16, 18, 20, 22, 11, 12, 13},
};

// One bit per channel is plenty for a text test: bit0=R, bit1=G, bit2=B.
enum { BLACK = 0, RED = 1, GREEN = 2, YELLOW = 3, BLUE = 4, MAGENTA = 5, CYAN = 6, WHITE = 7 };
static uint8_t fb[HEIGHT][WIDTH];

// 5x7 font, one byte per column, LSB at the top.
typedef struct { char ch; uint8_t col[5]; } glyph_t;
static const glyph_t font[] = {
    {' ', {0, 0, 0, 0, 0}}, {'!', {0, 0, 0x5f, 0, 0}}, {'.', {0, 0x60, 0x60, 0, 0}},
    {'0', {0x3e, 0x51, 0x49, 0x45, 0x3e}}, {'1', {0, 0x42, 0x7f, 0x40, 0}},
    {'2', {0x42, 0x61, 0x51, 0x49, 0x46}}, {'3', {0x21, 0x41, 0x45, 0x4b, 0x31}},
    {'4', {0x18, 0x14, 0x12, 0x7f, 0x10}}, {'5', {0x27, 0x45, 0x45, 0x45, 0x39}},
    {'6', {0x3c, 0x4a, 0x49, 0x49, 0x30}}, {'7', {1, 0x71, 9, 5, 3}},
    {'8', {0x36, 0x49, 0x49, 0x49, 0x36}}, {'9', {6, 0x49, 0x49, 0x29, 0x1e}},
    {'C', {0x3e, 0x41, 0x41, 0x41, 0x22}}, {'E', {0x7f, 0x49, 0x49, 0x49, 0x41}},
    {'H', {0x7f, 8, 8, 8, 0x7f}}, {'I', {0, 0x41, 0x7f, 0x41, 0}},
    {'L', {0x7f, 0x40, 0x40, 0x40, 0x40}}, {'O', {0x3e, 0x41, 0x41, 0x41, 0x3e}},
    {'P', {0x7f, 9, 9, 9, 6}}, {'V', {0x1f, 0x20, 0x40, 0x20, 0x1f}},
};

static const uint8_t *glyph(char c) {
    for (size_t i = 0; i < count_of(font); ++i)
        if (font[i].ch == c) return font[i].col;
    return font[0].col;
}

static void draw_text(const char *s, int x, int y, uint8_t colour) {
    for (; *s; ++s, x += 6) {
        const uint8_t *g = glyph(*s);
        for (int cx = 0; cx < 5; ++cx)
            for (int cy = 0; cy < 7; ++cy)
                if ((g[cx] >> cy) & 1 && x + cx >= 0 && x + cx < WIDTH && y + cy < HEIGHT)
                    fb[y + cy][x + cx] = colour;
    }
}

static void draw_frame(const char *label) {
    memset(fb, 0, sizeof(fb));
    // Border: verifies all four edges, both panel halves and the row order.
    for (int x = 0; x < WIDTH; ++x) fb[0][x] = fb[HEIGHT - 1][x] = YELLOW;
    for (int y = 0; y < HEIGHT; ++y) fb[y][0] = fb[y][WIDTH - 1] = YELLOW;
    draw_text("HELLO!", 14, 3, RED);    // top half only  -> R1
    draw_text("PICO", 20, 12, GREEN);   // straddles the halves -> G1 and G2
    draw_text(label, 20, 22, BLUE);     // bottom half only -> B2
}

static uint32_t all_mask(const pinout_t *p) {
    const uint pins[] = {p->r1, p->g1, p->b1, p->r2, p->g2, p->b2, p->a, p->b,
                         p->c, p->d, p->e, p->clk, p->lat, p->oe};
    uint32_t m = 0;
    for (size_t i = 0; i < count_of(pins); ++i) m |= 1u << pins[i];
    return m;
}

static inline void tiny_delay(void) { busy_wait_at_least_cycles(8); }

static inline void clock_pulse(const pinout_t *p) {
    tiny_delay();
    gpio_put(p->clk, 1);
    tiny_delay();
    gpio_put(p->clk, 0);
}

static inline uint32_t rgb_bits(const pinout_t *p, uint8_t top, uint8_t bottom) {
    return ((top & 1) ? 1u << p->r1 : 0) | ((top & 2) ? 1u << p->g1 : 0) |
           ((top & 4) ? 1u << p->b1 : 0) | ((bottom & 1) ? 1u << p->r2 : 0) |
           ((bottom & 2) ? 1u << p->g2 : 0) | ((bottom & 4) ? 1u << p->b2 : 0);
}

static void fm6126a_init(const pinout_t *p) {
    const uint32_t rgb_mask = rgb_bits(p, WHITE, WHITE);
    const uint16_t regs[2] = {0x7fff /* reg 12: full current */, 0x0040 /* reg 13: enable */};
    const int latch_cols[2] = {12, 13};
    gpio_put(p->oe, 1);
    for (int r = 0; r < 2; ++r) {
        for (int x = 0; x < WIDTH; ++x) {
            gpio_put_masked(rgb_mask, (regs[r] >> (x % 16)) & 1 ? rgb_mask : 0);
            gpio_put(p->lat, x >= WIDTH - latch_cols[r]);
            clock_pulse(p);
        }
        gpio_put(p->lat, 0);
    }
}

static void scan_once(const pinout_t *p) {
    const uint32_t rgb_mask = rgb_bits(p, WHITE, WHITE);
    const uint32_t addr_mask = (1u << p->a) | (1u << p->b) | (1u << p->c) |
                               (1u << p->d) | (1u << p->e);
    for (int row = 0; row < SCAN_ROWS; ++row) {
        for (int x = 0; x < WIDTH; ++x) {
            gpio_put_masked(rgb_mask, rgb_bits(p, fb[row][x], fb[row + SCAN_ROWS][x]));
            clock_pulse(p);
        }
        gpio_put(p->oe, 1);  // blank while switching rows
        uint32_t addr = ((row & 1) ? 1u << p->a : 0) | ((row & 2) ? 1u << p->b : 0) |
                        ((row & 4) ? 1u << p->c : 0) | ((row & 8) ? 1u << p->d : 0);
        gpio_put_masked(addr_mask, addr);  // E stays low on a 32-row panel
        tiny_delay();
        gpio_put(p->lat, 1);
        tiny_delay();
        gpio_put(p->lat, 0);
        gpio_put(p->oe, 0);
        sleep_us(OE_ON_US);
        gpio_put(p->oe, 1);
    }
}

int main(void) {
    stdio_init_all();
    gpio_init(PICO_DEFAULT_LED_PIN);
    gpio_set_dir(PICO_DEFAULT_LED_PIN, GPIO_OUT);

    const pinout_t *p = &pinouts[PINOUT];
    const uint32_t mask = all_mask(p);
    gpio_init_mask(mask);
    gpio_set_dir_out_masked(mask);
    gpio_put_masked(mask, 1u << p->oe);  // everything low except OE (blanked)

    fm6126a_init(p);
    draw_frame(p->name);
    while (true) {
        scan_once(p);
        gpio_put(PICO_DEFAULT_LED_PIN, (to_ms_since_boot(get_absolute_time()) / 500) & 1);
    }
}
