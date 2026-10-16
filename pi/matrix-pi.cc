// Raspberry Pi HUB75 backend for matrixd's existing framebuffer protocol.
#include <stdint.h>
#include <stdio.h>
#include <unistd.h>

#include "led-matrix.h"

using rgb_matrix::FrameCanvas;
using rgb_matrix::RGBMatrix;

static constexpr int kWidth = 64;
static constexpr int kHeight = 32;
static uint16_t pixels[kWidth * kHeight];
static uint8_t brightness = 128;

static bool read_exact(void *data, size_t size) {
  uint8_t *p = static_cast<uint8_t *>(data);
  while (size) {
    const ssize_t n = read(STDIN_FILENO, p, size);
    if (n <= 0) return false;
    p += n;
    size -= n;
  }
  return true;
}

static void draw(FrameCanvas *canvas) {
  if (!brightness) {
    canvas->Clear();
    return;
  }
  for (int y = 0; y < kHeight; ++y) {
    for (int x = 0; x < kWidth; ++x) {
      const uint16_t p = pixels[y * kWidth + x];
      canvas->SetPixel(x, y, ((p >> 8) & 15) * 17,
                       ((p >> 4) & 15) * 17, (p & 15) * 17);
    }
  }
}

int main(int argc, char **argv) {
  RGBMatrix::Options options;
  options.rows = kHeight;
  options.cols = kWidth;
  options.hardware_mapping = "regular";
  options.panel_type = "FM6126A";

  rgb_matrix::RuntimeOptions runtime;
  runtime.gpio_slowdown = 2;
  RGBMatrix *matrix = RGBMatrix::CreateFromFlags(&argc, &argv, &options, &runtime);
  if (!matrix) return 1;

  matrix->SetBrightness(50);
  FrameCanvas *canvas = matrix->CreateFrameCanvas();
  while (true) {
    const int command = getchar();
    if (command == EOF) break;
    if (command == 'P') {
      fputs("PICO-MATRIX 1 64 32\n", stdout); // Keep the established protocol.
    } else if (command == 'F') {
      if (!read_exact(pixels, sizeof(pixels))) break;
      draw(canvas);
      canvas = matrix->SwapOnVSync(canvas);
      fputs("K\n", stdout);
    } else if (command == 'B') {
      if (!read_exact(&brightness, 1)) break;
      matrix->SetBrightness(brightness ? (brightness * 99 / 255) + 1 : 1);
      draw(canvas);
      canvas = matrix->SwapOnVSync(canvas);
      fputs("K\n", stdout);
    }
    fflush(stdout);
  }
  matrix->Clear();
  delete matrix;
  return 0;
}
