package raytracing

import "testing"

func TestCPUShadePixelNearSphereTangent(t *testing.T) {
	benchmark := Benchmark{State: &State{Width: 64, Height: 512, Spheres: 64}}
	benchmark.genSpheres()

	// The gfx942 kernel hits sphere 55 here. Rounding the products before
	// adding them in the CPU reference instead gave this pixel the background.
	got := benchmark.cpuShadePixel(59, 265)
	want := [4]byte{37, 29, 21, 255}
	if got != want {
		t.Fatalf("shade pixel (59, 265) = %v, want %v", got, want)
	}
}
