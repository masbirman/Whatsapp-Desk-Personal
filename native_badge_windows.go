//go:build windows

package main

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Numeric unread overlay icons for the Windows taskbar (M4-04). The icon is
// rendered with GDI into a 32bpp DIB: a red disc with the unread count, with
// the alpha channel fixed up afterwards because GDI text output leaves the
// alpha bytes at zero. Any failure here falls back to the progress-bar badge
// in setTaskbarBadge.

var (
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procCreateBitmap       = gdi32.NewProc("CreateBitmap")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procCreateFontW        = gdi32.NewProc("CreateFontW")
	procSetBkMode          = gdi32.NewProc("SetBkMode")
	procSetTextColor       = gdi32.NewProc("SetTextColor")

	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procDrawTextW          = user32.NewProc("DrawTextW")
	procCreateIconIndirect = user32.NewProc("CreateIconIndirect")
)

const (
	badgeIconSize   = 16
	badgeCountLimit = 99
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32 // negative for top-down
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
	Colors [2]uint32
}

type iconInfo struct {
	FIcon    uint32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

type drawTextRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

var (
	overlayIconsMu sync.Mutex
	overlayIcons   = map[int]uintptr{}
)

func unreadOverlayText(count int) string {
	if count > badgeCountLimit {
		count = badgeCountLimit
	}
	return fmt.Sprintf("%d", count)
}

func createUnreadOverlayIcon(count int) (uintptr, error) {
	overlayIconsMu.Lock()
	defer overlayIconsMu.Unlock()
	if hicon, ok := overlayIcons[count]; ok {
		return hicon, nil
	}
	hicon, err := renderUnreadOverlayIcon(count)
	if err != nil {
		return 0, err
	}
	overlayIcons[count] = hicon
	return hicon, nil
}

func renderUnreadOverlayIcon(count int) (uintptr, error) {
	text := unreadOverlayText(count)
	textUTF16, err := syscall.UTF16FromString(text)
	if err != nil {
		return 0, err
	}
	face, err := syscall.UTF16FromString("Segoe UI")
	if err != nil {
		return 0, err
	}

	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return 0, fmt.Errorf("GetDC failed")
	}
	defer procReleaseDC.Call(0, hdcScreen)

	hdc, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	if hdc == 0 {
		return 0, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(hdc)

	bmi := bitmapInfo{Header: bitmapInfoHeader{
		Size:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:    badgeIconSize,
		Height:   -badgeIconSize,
		Planes:   1,
		BitCount: 32,
	}}
	var bitsAddr uintptr
	hbmColor, _, _ := procCreateDIBSection.Call(hdc, uintptr(unsafe.Pointer(&bmi)), 0, uintptr(unsafe.Pointer(&bitsAddr)), 0, 0)
	if hbmColor == 0 || bitsAddr == 0 {
		return 0, fmt.Errorf("CreateDIBSection failed")
	}
	defer procDeleteObject.Call(hbmColor)

	oldBmp, _, _ := procSelectObject.Call(hdc, hbmColor)
	defer procSelectObject.Call(hdc, oldBmp)

	// Red disc, transparent corners: write pixels directly so the alpha
	// channel starts fully correct for the background shape.
	pixels := unsafe.Slice((*uint32)(unsafe.Pointer(*(*unsafe.Pointer)(unsafe.Pointer(&bitsAddr)))), badgeIconSize*badgeIconSize)
	center := float64(badgeIconSize) / 2
	radius := center - 0.5
	const opaqueRed = 0xFF0000FF // memory order BGRA: alpha=FF, blue=00, green=00, red=FF
	for y := 0; y < badgeIconSize; y++ {
		for x := 0; x < badgeIconSize; x++ {
			dx := float64(x) + 0.5 - center
			dy := float64(y) + 0.5 - center
			if dx*dx+dy*dy <= radius*radius {
				pixels[y*badgeIconSize+x] = opaqueRed
			}
		}
	}

	fontHeight := int32(-10)
	hfont, _, _ := procCreateFontW.Call(
		uintptr(fontHeight), 0, 0, 0, 700, // negative height = char height, bold
		0, 0, 0, 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&face[0])),
	)
	if hfont != 0 {
		defer procDeleteObject.Call(hfont)
		oldFont, _, _ := procSelectObject.Call(hdc, hfont)
		defer procSelectObject.Call(hdc, oldFont)
	}
	procSetBkMode.Call(hdc, 1) // TRANSPARENT
	procSetTextColor.Call(hdc, 0x00FFFFFF)

	rect := drawTextRect{Left: 0, Top: 0, Right: badgeIconSize, Bottom: badgeIconSize}
	const dtCenter = 0x1
	const dtVCenter = 0x4
	const dtSingleLine = 0x20
	ret, _, _ := procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&textUTF16[0])), uintptr(len(textUTF16)), uintptr(unsafe.Pointer(&rect)), dtCenter|dtVCenter|dtSingleLine)
	if ret == 0 {
		return 0, fmt.Errorf("DrawTextW failed")
	}

	// GDI wrote RGB but left alpha at zero; make every drawn pixel opaque.
	for i := range pixels {
		if pixels[i] != 0 {
			pixels[i] |= 0xFF000000
		}
	}

	hbmMask, _, _ := procCreateBitmap.Call(badgeIconSize, badgeIconSize, 1, 1, 0)
	if hbmMask == 0 {
		return 0, fmt.Errorf("CreateBitmap mask failed")
	}
	defer procDeleteObject.Call(hbmMask)

	info := iconInfo{FIcon: 1, HbmMask: hbmMask, HbmColor: hbmColor}
	hicon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&info)))
	if hicon == 0 {
		return 0, fmt.Errorf("CreateIconIndirect failed")
	}
	return hicon, nil
}
