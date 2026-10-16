#include "pico/stdlib.h"
#include "pico/cyw43_arch.h"

int main(void) {
    if (cyw43_arch_init()) while (true) tight_loop_contents();
    while (true) {
        cyw43_arch_gpio_put(CYW43_WL_GPIO_LED_PIN, 1);
        sleep_ms(500);
        cyw43_arch_gpio_put(CYW43_WL_GPIO_LED_PIN, 0);
        sleep_ms(500);
    }
}
