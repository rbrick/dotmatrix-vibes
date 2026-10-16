#include "pico/stdlib.h"
#include "hardware/gpio.h"
#ifdef CYW43_WL_GPIO_LED_PIN
#include "pico/cyw43_arch.h"
#endif

// Independent HUB75 GPIO test: no PIO, DMA, framebuffer, or serial required.
// Alternate Pico adapter pinout supplied by user; testing, not yet verified
// against the Seengreat v3.9 schematic.
static const int rgb_pins[6] = {2, 3, 4, 5, 8, 9};
static const int addr_pins[4] = {10, 16, 18, 20};
#define CLK 11
#define LAT 12
#define OE 13

static void row(int address, int colour) {
    gpio_put(OE, 1);                 // Blank while shifting and latching.
    for (int x = 0; x < 64; ++x) {
        // Same colour in both halves. Moving stripe verifies clock/data.
        int c = (x >= 4 && x < 60) ? colour : 0;
        for (int i = 0; i < 6; ++i) gpio_put(rgb_pins[i], (c >> (i % 3)) & 1);
        sleep_us(1);
        gpio_put(CLK, 1);
        sleep_us(1);
        gpio_put(CLK, 0);
    }
    for (int i = 0; i < 4; ++i) gpio_put(addr_pins[i], (address >> i) & 1);
    sleep_us(2);
    gpio_put(LAT, 1);
    sleep_us(2);
    gpio_put(LAT, 0);
    gpio_put(OE, 0);
    sleep_us(150);                   // ~5% duty, modest power draw.
    gpio_put(OE, 1);
}

int main(void) {
    const int pins[] = {2, 3, 4, 5, 8, 9, 10, 16, 18, 20, CLK, LAT, OE};
    for (int i = 0; i < sizeof(pins) / sizeof(pins[0]); ++i) {
        int p = pins[i];
        gpio_init(p);
        gpio_set_dir(p, GPIO_OUT);
        gpio_put(p, 0);
    }
    gpio_put(OE, 1);
#ifdef CYW43_WL_GPIO_LED_PIN
    if (cyw43_arch_init()) while (true) tight_loop_contents();
#elif defined(PICO_DEFAULT_LED_PIN)
    gpio_init(PICO_DEFAULT_LED_PIN);
    gpio_set_dir(PICO_DEFAULT_LED_PIN, GPIO_OUT);
#endif
    while (true) {
#ifdef CYW43_WL_GPIO_LED_PIN
        cyw43_arch_gpio_put(CYW43_WL_GPIO_LED_PIN, (to_ms_since_boot(get_absolute_time()) / 500) & 1);
#elif defined(PICO_DEFAULT_LED_PIN)
        gpio_put(PICO_DEFAULT_LED_PIN, (to_ms_since_boot(get_absolute_time()) / 500) & 1);
#endif
        for (int a = 0; a < 16; ++a) {
            int colour = a < 5 ? 1 : a < 10 ? 2 : a < 15 ? 4 : 7;
            row(a, colour);
        }
    }
}
