RGB_MATRIX_DIR ?= /opt/rpi-rgb-led-matrix
CXXFLAGS := -O3 -Wall -Wextra -std=c++11 -I$(RGB_MATRIX_DIR)/include
LDLIBS := $(RGB_MATRIX_DIR)/lib/librgbmatrix.a -lrt -lm -lpthread

matrix-pi: matrix-pi.cc
	$(MAKE) -C $(RGB_MATRIX_DIR)/lib
	$(CXX) $(CXXFLAGS) -o $@ $< $(LDLIBS)

install: matrix-pi
	install -m 0755 matrix-pi /usr/local/bin/matrix-pi

clean:
	rm -f matrix-pi

.PHONY: install clean
