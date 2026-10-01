//go:build windows

package main

import (
	"bytes"
	"image"
	"image/png"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// Windows system tray (M4-05): Shell_NotifyIcon on a dedicated thread with
// its own message loop. Menu actions Open/Lock run on the tray thread;
// page-touching actions are marshalled through w.Dispatch by the caller-
// supplied action closures. Tray failures never crash the app: if the tray
// cannot be created, the function returns and the app runs without it.

const (
	trayCallbackMessage = 0x8000 + 0x0A01 // WM_APP range, private use
	trayUpdateTipMsg    = 0x8000 + 0x0A02
	trayRemoveMsg       = 0x8000 + 0x0A03

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2

	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4

	tpmRightButton = 0x2
	tpmReturnCmd   = 0x100

	mfString    = 0x0
	mfSeparator = 0x800
	mfChecked   = 0x8

	gwlpWndProc    = -4
	wmClose        = 0x10
	wmDestroy      = 0x2
	wmNull         = 0x0
	wmSize         = 0x5
	wmCommand      = 0x111
	sizeMinimized  = 1
	swHide         = 0
	swRestore      = 9
	swShow         = 5
	dwExStyleNone  = 0
	dwStyleMessage = 0x80000000 // WS_POPUP: never visible

	wmLButtonUp    = 0x202
	wmRButtonDown  = 0x204
	wmRButtonUp    = 0x205
)

// Tray menu command IDs (index into trayMenuItems order).
const (
	trayCmdOpen = 1 + iota
	trayCmdPrivacy
	trayCmdLock
	trayCmdNotifications
	trayCmdSettings
	trayCmdQuit
)

type nativeTrayActions struct {
	// Open restores and focuses the main window; it must respect the app
	// lock and never reveal a hidden window.
	Open func()
	// Privacy toggles the page privacy mode.
	Privacy func()
	// Lock engages the native app lock.
	Lock func()
	// Notifications toggles the notification policy; receives the current
	// state and returns the new one.
	Notifications func(currentlyEnabled bool) bool
	// Settings opens the Control Center.
	Settings func()
	// Quit exits the application.
	Quit func()
	// Tooltip builds the tray tooltip text (no message content).
	Tooltip func() string
	// NotificationsEnabled reports the current policy state for the checkbox.
	NotificationsEnabled func() bool
}

type notifyIconDataW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [128]uint16
	UVersion         uint32
	SzInfoTitle      [256]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type windowClassW struct {
	Size         uint32
	Style        uint32
	WndProc      uintptr
	ClsExtra     int32
	WndExtra     int32
	Instance     uintptr
	Icon         uintptr
	Cursor       uintptr
	Background   uintptr
	MenuName     uintptr
	ClassName    uintptr
	SmallIcon    uintptr
}

type wndProcPoint struct{ X, Y int32 }

var (
	procShellNotifyIcon      = shell32.NewProc("Shell_NotifyIconW")
	procRegisterWindowMsg    = user32.NewProc("RegisterWindowMessageW")
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostMessage          = user32.NewProc("PostMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procCreatePopupMenu      = user32.NewProc("CreatePopupMenu")
	procAppendMenuW          = user32.NewProc("AppendMenuW")
	procTrackPopupMenu       = user32.NewProc("TrackPopupMenu")
	procDestroyMenu          = user32.NewProc("DestroyMenu")
	procSetForegroundWindow2 = user32.NewProc("SetForegroundWindow")
	procGetCursorPos         = user32.NewProc("GetCursorPos")
	procGetSystemMetrics     = user32.NewProc("GetSystemMetrics")

	trayHwnd          atomic.Uintptr
	trayQuitRequested atomic.Bool
	windowTrayPrevWnd atomic.Uintptr
	windowTrayHooked  atomic.Bool
	mainHwndRef       atomic.Uintptr
	trayTaskbarMsg    atomic.Uint32
)

// Win32 message-loop callbacks must be created once per process.
var (
	trayWndProcCallback          = syscall.NewCallback(trayWindowProc)
	mainWindowTrayProcCallback   = syscall.NewCallback(mainWindowTrayWndProc)
)

func shellNotifyIcon(dwMessage uint32, data *notifyIconDataW) bool {
	ret, _, _ := procShellNotifyIcon.Call(uintptr(dwMessage), uintptr(unsafe.Pointer(data)))
	return ret != 0
}

func setTrayTip(data *notifyIconDataW, text string) {
	tip, err := syscall.UTF16FromString(text)
	if err != nil {
		tip = []uint16{0}
	}
	if len(tip) > len(data.SzTip) {
		tip = tip[:len(data.SzTip)-1]
		tip = append(tip, 0)
	}
	copy(data.SzTip[:], tip)
}

func trayWindowProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case trayCallbackMessage:
		// lParam carries the mouse message for NOTIFYICONDATA callbacks.
		switch lParam {
		case wmLButtonUp:
			if trayActionsGlobal.Open != nil {
				trayActionsGlobal.Open()
			}
			return 0
		case wmRButtonDown, wmRButtonUp:
			showTrayMenu(hwnd)
			return 0
		}
		return 0
	case trayUpdateTipMsg:
		refreshTrayTooltip()
		return 0
	case trayRemoveMsg:
		removeTrayIcon(hwnd)
		procPostQuitMessage.Call(0)
		return 0
	case wmDestroy:
		removeTrayIcon(hwnd)
		procPostQuitMessage.Call(0)
		return 0
	}
	if code := trayTaskbarMsg.Load(); code != 0 && uint32(msg) == code {
		// Explorer restarted: re-add the icon.
		addTrayIcon(hwnd)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

var trayActionsGlobal nativeTrayActions

func addTrayIcon(hwnd uintptr) bool {
	hicon := trayIconHandle()
	data := notifyIconDataW{
		CbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
		HWnd:             hwnd,
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: trayCallbackMessage,
		HIcon:            hicon,
	}
	if trayActionsGlobal.Tooltip != nil {
		setTrayTip(&data, trayActionsGlobal.Tooltip())
	} else {
		setTrayTip(&data, "WhatsApp Desk")
	}
	return shellNotifyIcon(nimAdd, &data)
}

func removeTrayIcon(hwnd uintptr) {
	data := notifyIconDataW{
		CbSize: uint32(unsafe.Sizeof(notifyIconDataW{})),
		HWnd:   hwnd,
		UID:    1,
	}
	shellNotifyIcon(nimDelete, &data)
}

func refreshTrayTooltip() {
	hwnd := trayHwnd.Load()
	if hwnd == 0 {
		return
	}
	text := "WhatsApp Desk"
	if trayActionsGlobal.Tooltip != nil {
		text = trayActionsGlobal.Tooltip()
	}
	data := notifyIconDataW{
		CbSize:   uint32(unsafe.Sizeof(notifyIconDataW{})),
		HWnd:     hwnd,
		UID:      1,
		UFlags:   nifTip,
	}
	setTrayTip(&data, text)
	shellNotifyIcon(nimModify, &data)
}

func showTrayMenu(hwnd uintptr) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	appendTrayMenu := func(flags uintptr, id uintptr, text string) {
		t, _ := syscall.UTF16FromString(text)
		procAppendMenuW.Call(menu, flags, id, uintptr(unsafe.Pointer(&t[0])))
	}
	appendTrayMenu(mfString, trayCmdOpen, "Open")
	appendTrayMenu(mfSeparator, 0, "")
	appendTrayMenu(mfString, trayCmdPrivacy, "Toggle privacy mode")
	appendTrayMenu(mfString, trayCmdLock, "Lock now")
	notifFlags := uintptr(mfString)
	if trayActionsGlobal.NotificationsEnabled != nil && trayActionsGlobal.NotificationsEnabled() {
		notifFlags |= mfChecked
	}
	appendTrayMenu(notifFlags, trayCmdNotifications, "Desktop notifications")
	appendTrayMenu(mfSeparator, 0, "")
	appendTrayMenu(mfString, trayCmdSettings, "Control Center")
	appendTrayMenu(mfSeparator, 0, "")
	appendTrayMenu(mfString, trayCmdQuit, "Quit")

	// TrackPopupMenu needs the foreground window for correct dismissal.
	procSetForegroundWindow2.Call(hwnd)
	var pt wndProcPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	// TPM_RETURNCMD returns the chosen command directly.
	id, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmReturnCmd, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	procPostMessage.Call(hwnd, wmNull, 0, 0)
	runTrayCommand(uintptr(id))
}

func runTrayCommand(id uintptr) {
	switch id {
	case trayCmdOpen:
		if trayActionsGlobal.Open != nil {
			trayActionsGlobal.Open()
		}
	case trayCmdPrivacy:
		if trayActionsGlobal.Privacy != nil {
			trayActionsGlobal.Privacy()
		}
	case trayCmdLock:
		if trayActionsGlobal.Lock != nil {
			trayActionsGlobal.Lock()
		}
	case trayCmdNotifications:
		if trayActionsGlobal.Notifications != nil {
			trayActionsGlobal.Notifications(trayActionsGlobal.NotificationsEnabled())
			refreshTrayTooltip()
		}
	case trayCmdSettings:
		if trayActionsGlobal.Settings != nil {
			trayActionsGlobal.Settings()
		}
	case trayCmdQuit:
		if trayActionsGlobal.Quit != nil {
			trayActionsGlobal.Quit()
		}
	}
}

func trayThread(actions nativeTrayActions, iconPNG []byte) {
	// Win32 windows and their message pumps are thread-bound.
	runtime.LockOSThread()
	trayIconSource = iconPNG
	defer runtime.UnlockOSThread()

	trayActionsGlobal = actions
	className, _ := syscall.UTF16FromString("WhatsAppDeskTrayHost")
	var wc windowClassW
	wc.Size = uint32(unsafe.Sizeof(wc))
	wc.WndProc = trayWndProcCallback
	wc.Instance = 0
	wc.ClassName = uintptr(unsafe.Pointer(&className[0]))
	// RegisterClassExW errors are ignored: the class may already exist.
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	instanceName, _ := syscall.UTF16FromString("WhatsAppDeskTrayWindow")
	hwnd, _, _ := procCreateWindowExW.Call(
		dwExStyleNone, uintptr(unsafe.Pointer(&className[0])), uintptr(unsafe.Pointer(&instanceName[0])),
		dwStyleMessage, 0, 0, 0, 0, 0, 0, 0, 0,
	)
	if hwnd == 0 {
		return // Tray unavailable; the app continues without it.
	}
	trayHwnd.Store(hwnd)

	// Explorer broadcasts TaskbarCreated when the shell restarts.
	msg, _, _ := procRegisterWindowMsg.Call(uintptr(unsafe.Pointer(taskbarCreatedStringPtr())))
	if msg != 0 {
		trayTaskbarMsg.Store(uint32(msg))
	}

	if !addTrayIcon(hwnd) {
		trayHwnd.Store(0)
		procDestroyWindow.Call(hwnd)
		return
	}

	var msgStruct [7]uintptr // MSG layout on 64-bit: hwnd, message, wParam, lParam, time, pt(2), private
	for {
		ret, _, _ := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&msgStruct[0])), 0, 0, 0,
		)
		if ret == 0 || int32(ret) == -1 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msgStruct[0])))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msgStruct[0])))
	}
	trayHwnd.Store(0)
}

// trayIconHandle decodes the embedded app icon once and renders a small
// tray-sized HICON. Failure yields 0, in which case Windows shows a default
// blank icon but the tray still works.
func trayIconHandle() uintptr {
	trayIconOnce.Do(func() {
		src, err := png.Decode(bytes.NewReader(trayIconSource))
		if err != nil {
			return
		}
		trayIconHICON = renderImageAsSmallIcon(src)
	})
	return trayIconHICON
}

var (
	trayIconOnce   sync.Once
	trayIconHICON  uintptr
	trayIconSource []byte
)

func renderImageAsSmallIcon(src image.Image) uintptr {
	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return 0
	}
	defer procReleaseDC.Call(0, hdcScreen)
	hdc, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	if hdc == 0 {
		return 0
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
		return 0
	}
	defer procDeleteObject.Call(hbmColor)

	oldBmp, _, _ := procSelectObject.Call(hdc, hbmColor)
	defer procSelectObject.Call(hdc, oldBmp)

	pixels := unsafe.Slice((*uint32)(unsafe.Pointer(*(*unsafe.Pointer)(unsafe.Pointer(&bitsAddr)))), badgeIconSize*badgeIconSize)
	bounds := src.Bounds()
	sx := float64(bounds.Dx()) / badgeIconSize
	sy := float64(bounds.Dy()) / badgeIconSize
	for y := 0; y < badgeIconSize; y++ {
		for x := 0; x < badgeIconSize; x++ {
			px := bounds.Min.X + int(float64(x)*sx)
			py := bounds.Min.Y + int(float64(y)*sy)
			r, g, b, a := src.At(px, py).RGBA()
			if a == 0 {
				continue
			}
			// Premultiplied BGRA expected by a 32bpp icon color bitmap.
			pr := uint8(r >> 8)
			pg := uint8(g >> 8)
			pb := uint8(b >> 8)
			pa := uint8(a >> 8)
			if pa != 0xFF {
				pr = uint8(uint16(pr) * uint16(pa) / 255)
				pg = uint8(uint16(pg) * uint16(pa) / 255)
				pb = uint8(uint16(pb) * uint16(pa) / 255)
			}
			pixels[y*badgeIconSize+x] = uint32(pa)<<24 | uint32(pb)<<16 | uint32(pg)<<8 | uint32(pr)
		}
	}

	hbmMask, _, _ := procCreateBitmap.Call(badgeIconSize, badgeIconSize, 1, 1, 0)
	if hbmMask == 0 {
		return 0
	}
	defer procDeleteObject.Call(hbmMask)

	info := iconInfo{FIcon: 1, HbmMask: hbmMask, HbmColor: hbmColor}
	hicon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&info)))
	return hicon
}

var taskbarCreatedOnce []uint16

func taskbarCreatedStringPtr() *uint16 {
	if taskbarCreatedOnce == nil {
		s, _ := syscall.UTF16FromString("TaskbarCreated")
		taskbarCreatedOnce = s
	}
	return &taskbarCreatedOnce[0]
}

// startWindowsTray launches the tray thread. It never blocks the caller and
// never fails the app: if the tray cannot start, the app runs without it.
func startWindowsTray(actions nativeTrayActions, iconPNG []byte) {
	go trayThread(actions, iconPNG)
}

func updateWindowsTrayTooltip() {
	if hwnd := trayHwnd.Load(); hwnd != 0 {
		procPostMessage.Call(hwnd, trayUpdateTipMsg, 0, 0)
	}
}

func stopWindowsTray() {
	if hwnd := trayHwnd.Load(); hwnd != 0 {
		procPostMessage.Call(hwnd, trayRemoveMsg, 0, 0)
	}
}

// --- Main window minimize/close-to-tray hook -------------------------------

// mainWindowTrayWndProc intercepts minimize and close for tray behavior.
// Everything else is forwarded to the original webview window procedure.
func mainWindowTrayWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	prev := windowTrayPrevWnd.Load()
	switch msg {
	case wmSize:
		if wParam == sizeMinimized && minimizeToTrayEnabled() {
			procShowNormal.Call(hwnd, swHide)
			return 0
		}
	case wmClose:
		if closeToTrayEnabled() && !trayQuitRequested.Load() {
			procShowNormal.Call(hwnd, swHide)
			return 0
		}
	}
	if prev != 0 {
		ret, _, _ := procCallWindowProc.Call(prev, hwnd, msg, wParam, lParam)
		return ret
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

func installMainWindowTrayHook(hwnd uintptr) bool {
	if windowTrayHooked.Load() {
		return true
	}
	procSetWindowLongPtr := user32.NewProc("SetWindowLongPtrW")
	gwlpWndProcValue := int32(gwlpWndProc) // -4, sign-extended at conversion
	prev, _, _ := procSetWindowLongPtr.Call(hwnd, uintptr(gwlpWndProcValue), mainWindowTrayProcCallback)
	if prev == 0 {
		return false
	}
	windowTrayPrevWnd.Store(prev)
	windowTrayHooked.Store(true)
	mainHwndRef.Store(hwnd)
	return true
}

var procCallWindowProc = user32.NewProc("CallWindowProcW")

func minimizeToTrayEnabled() bool {
	return loadSettings().MinimizeToTray
}

func closeToTrayEnabled() bool {
	return loadSettings().CloseToTray
}
