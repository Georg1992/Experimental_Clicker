//go:build windows

package runner

import (
	"fmt"
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	gdi32                = windows.NewLazySystemDLL("gdi32.dll")
	procGetDC            = user32.NewProc("GetDC")
	procReleaseDC        = user32.NewProc("ReleaseDC")
	procCreateCompatDC   = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatBmp  = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject     = gdi32.NewProc("SelectObject")
	procBitBlt           = gdi32.NewProc("BitBlt")
	procDeleteDC         = gdi32.NewProc("DeleteDC")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procGetDIBits        = gdi32.NewProc("GetDIBits")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

const gdiSrcCopy = 0x00CC0020

// ScreenSize returns the primary monitor dimensions.
func ScreenSize() (w, h int) {
	sw, _, _ := procGetSystemMetrics.Call(0)
	sh, _, _ := procGetSystemMetrics.Call(1)
	return int(sw), int(sh)
}

// CaptureScreenRegion grabs a screen rectangle into an RGBA image.
func CaptureScreenRegion(roi BarROI) (*image.RGBA, error) {
	if roi.W <= 0 || roi.H <= 0 {
		return nil, fmt.Errorf("invalid capture roi %v", roi)
	}

	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return nil, fmt.Errorf("GetDC failed")
	}
	defer procReleaseDC.Call(0, hdcScreen)

	hdcMem, _, _ := procCreateCompatDC.Call(hdcScreen)
	if hdcMem == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(hdcMem)

	hbmp, _, _ := procCreateCompatBmp.Call(hdcScreen, uintptr(roi.W), uintptr(roi.H))
	if hbmp == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(hbmp)

	old, _, _ := procSelectObject.Call(hdcMem, hbmp)
	defer procSelectObject.Call(hdcMem, old)

	r, _, _ := procBitBlt.Call(
		hdcMem, 0, 0, uintptr(roi.W), uintptr(roi.H),
		hdcScreen, uintptr(roi.X), uintptr(roi.Y), gdiSrcCopy,
	)
	if r == 0 {
		return nil, fmt.Errorf("BitBlt failed")
	}

	out := image.NewRGBA(image.Rect(0, 0, roi.W, roi.H))
	var bmi bitmapInfo
	bmi.Header.Size = uint32(unsafe.Sizeof(bmi.Header))
	bmi.Header.Width = int32(roi.W)
	bmi.Header.Height = -int32(roi.H)
	bmi.Header.Planes = 1
	bmi.Header.BitCount = 32
	bmi.Header.Compression = 0

	ptr := unsafe.Pointer(&out.Pix[0])
	ret, _, _ := procGetDIBits.Call(
		hdcMem, hbmp, 0, uintptr(roi.H),
		uintptr(ptr), uintptr(unsafe.Pointer(&bmi)), 0,
	)
	if ret == 0 {
		return nil, fmt.Errorf("GetDIBits failed")
	}

	// GetDIBits returns BGRA; convert to RGBA.
	for i := 0; i < len(out.Pix); i += 4 {
		out.Pix[i+0], out.Pix[i+2] = out.Pix[i+2], out.Pix[i+0]
	}
	return out, nil
}

// CapturePlayerBarSearch grabs the center region below the character.
func CapturePlayerBarSearch() (img *image.RGBA, roi BarROI, err error) {
	sw, sh := ScreenSize()
	roi = PlayerBarSearchROI(sw, sh)
	img, err = CaptureScreenRegion(roi)
	return img, roi, err
}

// BarROI defines a rectangular region for screen capture.
type BarROI struct {
	X, Y, W, H int
}

// PlayerBarSearchROI returns the region below the player’s status bar area.
func PlayerBarSearchROI(sw, sh int) BarROI {
	width, height := 200, 40
	x := (sw - width) / 2
	y := sh/2 + 200
	return BarROI{X: x, Y: y, W: width, H: height}
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
}
