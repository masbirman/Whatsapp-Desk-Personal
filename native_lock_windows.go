//go:build windows

package main

import (
	"syscall"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"
)

const (
	lockWSOverlapped    = 0x00000000
	lockWSCaption       = 0x00C00000
	lockWSSysMenu       = 0x00080000
	lockWSVisible       = 0x10000000
	lockWSChild         = 0x40000000
	lockWSBorder        = 0x00800000
	lockWSTabStop       = 0x00010000
	lockWSVScroll       = 0x00200000
	lockWSExTopmost     = 0x00000008
	lockWSExDlgFrame    = 0x00000001
	lockEditPassword    = 0x00000020
	lockEditAutoH       = 0x00000080
	lockButtonDefault   = 0x00000001
	lockButton          = 0x00000000
	lockSSLeft          = 0x00000000
	lockSWHide          = 0
	lockSWShow          = 5
	lockSWRestore       = 9
	lockPMRemove        = 0x0001
	lockWMCommand       = 0x0111
	lockWMClose         = 0x0010
	lockWMDestroy       = 0x0002
	lockWMKeyDown       = 0x0100
	lockVKReturn        = 0x000D
	lockDlgOK           = 1001
	lockDlgCancel       = 1002
	lockIDOK            = 1
	lockIDCancel        = 2
	lockIDYes           = 6
	lockIDNo            = 7
	lockMBOK            = 0x00000000
	lockMBYesNoCancel   = 0x00000003
	lockMBIconInfo      = 0x00000040
	lockMBIconWarn      = 0x00000030
	lockMBSetForeground = 0x00010000
	lockMBTopmost       = 0x00040000
	lockSMCXScreen      = 0
	lockSMCYScreen      = 1
)

var (
	procLockCreateWindowEx  = user32.NewProc("CreateWindowExW")
	procLockRegisterClass   = user32.NewProc("RegisterClassW")
	procLockDefWindowProc   = user32.NewProc("DefWindowProcW")
	procLockDestroyWindow   = user32.NewProc("DestroyWindow")
	procLockEnableWindow    = user32.NewProc("EnableWindow")
	procLockSetFocus        = user32.NewProc("SetFocus")
	procLockSetWindowText   = user32.NewProc("SetWindowTextW")
	procLockGetWindowText   = user32.NewProc("GetWindowTextW")
	procLockGetTextLength   = user32.NewProc("GetWindowTextLengthW")
	procLockShowWindow      = user32.NewProc("ShowWindow")
	procLockGetMessage      = user32.NewProc("GetMessageW")
	procLockPeekMessage     = user32.NewProc("PeekMessageW")
	procLockTranslate       = user32.NewProc("TranslateMessage")
	procLockDispatch        = user32.NewProc("DispatchMessageW")
	procLockIsDialogMessage = user32.NewProc("IsDialogMessageW")
	procLockWaitMessage     = user32.NewProc("WaitMessage")
	procLockPostQuit        = user32.NewProc("PostQuitMessage")
	procLockSendCommand     = user32.NewProc("SendMessageW")
	procLockMessageBox      = user32.NewProc("MessageBoxW")
	procLockGetMetrics      = user32.NewProc("GetSystemMetrics")
	procLockGetTickCount    = kernel32.NewProc("GetTickCount")
	procLockLastInput       = user32.NewProc("GetLastInputInfo")
	lockPromptProc          = syscall.NewCallback(nativeLockPromptWndProc)
	lockPromptClassName     = syscall.StringToUTF16Ptr("WhatsAppDeskNativeLockPrompt")
	lockPromptInstance      uintptr
	lockPromptState         struct {
		done     bool
		accepted bool
		edit     uintptr
		text     []uint16
	}
)

type nativeLockWndClass struct {
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
}

type nativeLockPoint struct{ X, Y int32 }
type nativeLockMessage struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   nativeLockPoint
	Private uint32
}
type nativeLastInputInfo struct {
	Size uint32
	Time uint32
}

func ensureNativeLockPromptClass() {
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	lockPromptInstance = instance
	class := nativeLockWndClass{
		Style: 0x0002 | 0x0001, WndProc: lockPromptProc,
		Instance: instance, ClassName: lockPromptClassName,
	}
	procLockRegisterClass.Call(uintptr(unsafe.Pointer(&class)))
}

func nativeLockPromptWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case lockWMCommand:
		command := uint16(wParam & 0xffff)
		if command == lockDlgOK {
			length, _, _ := procLockGetTextLength.Call(lockPromptState.edit)
			buffer := make([]uint16, int(length)+1)
			procLockGetWindowText.Call(lockPromptState.edit, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
			lockPromptState.text = buffer[:int(length)]
			lockPromptState.accepted = true
			lockPromptState.done = true
			procLockDestroyWindow.Call(hwnd)
			return 0
		}
		if command == lockDlgCancel {
			lockPromptState.accepted = false
			lockPromptState.done = true
			procLockDestroyWindow.Call(hwnd)
			return 0
		}
	case lockWMKeyDown:
		if wParam == lockVKReturn {
			procLockSendCommand.Call(hwnd, lockWMCommand, lockDlgOK, 0)
			return 0
		}
	case lockWMClose:
		lockPromptState.accepted = false
		lockPromptState.done = true
		procLockDestroyWindow.Call(hwnd)
		return 0
	case lockWMDestroy:
		return 0
	}
	result, _, _ := procLockDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func createNativePromptControl(parent uintptr, class, text string, style uint32, x, y, width, height int32, id int) uintptr {
	classPtr := syscall.StringToUTF16Ptr(class)
	textPtr := syscall.StringToUTF16Ptr(text)
	hwnd, _, _ := procLockCreateWindowEx.Call(
		0, uintptr(unsafe.Pointer(classPtr)), uintptr(unsafe.Pointer(textPtr)),
		uintptr(lockWSChild|lockWSVisible|style), uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		parent, uintptr(id), lockPromptInstance, 0,
	)
	return hwnd
}

func nativeCredentialPrompt(owner uintptr, title, message string, password bool) ([]byte, bool) {
	ensureNativeLockPromptClass()
	lockPromptState.done, lockPromptState.accepted, lockPromptState.edit = false, false, 0
	for i := range lockPromptState.text {
		lockPromptState.text[i] = 0
	}
	lockPromptState.text = nil
	if owner != 0 {
		procLockEnableWindow.Call(owner, 0)
		defer procLockEnableWindow.Call(owner, 1)
	}
	titlePtr := syscall.StringToUTF16Ptr(title)
	left, _, _ := procLockGetMetrics.Call(lockSMCXScreen)
	top, _, _ := procLockGetMetrics.Call(lockSMCYScreen)
	x, y := int32((int(left)-440)/2), int32((int(top)-220)/2)
	style := uint32(lockWSVisible | lockWSCaption | lockWSSysMenu)
	window, _, _ := procLockCreateWindowEx.Call(
		lockWSExTopmost|lockWSExDlgFrame, uintptr(unsafe.Pointer(lockPromptClassName)), uintptr(unsafe.Pointer(titlePtr)),
		uintptr(style), uintptr(x), uintptr(y), 440, 220, owner, 0, lockPromptInstance, 0,
	)
	if window == 0 {
		return nil, false
	}
	createNativePromptControl(window, "STATIC", "WhatsApp Desk", lockSSLeft, 24, 16, 380, 24, 0)
	createNativePromptControl(window, "STATIC", message, lockWSVScroll, 24, 46, 380, 70, 0)
	editStyle := uint32(lockWSBorder | lockWSTabStop | lockEditAutoH)
	if password {
		editStyle |= lockEditPassword
	}
	lockPromptState.edit = createNativePromptControl(window, "EDIT", "", editStyle, 24, 122, 380, 26, 0)
	createNativePromptControl(window, "BUTTON", "Continue", lockButtonDefault|lockWSTabStop, 236, 162, 80, 28, lockDlgOK)
	createNativePromptControl(window, "BUTTON", "Cancel", lockButton|lockWSTabStop, 324, 162, 80, 28, lockDlgCancel)
	procLockSetFocus.Call(lockPromptState.edit)
	for !lockPromptState.done {
		var message nativeLockMessage
		found, _, _ := procLockPeekMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, lockPMRemove)
		if found != 0 {
			if message.Message == 0x0012 { // WM_QUIT: preserve app shutdown after leaving the modal dialog.
				lockPromptState.done = true
				procLockPostQuit.Call(uintptr(message.WParam))
				break
			}
			isDialogMessage, _, _ := procLockIsDialogMessage.Call(window, uintptr(unsafe.Pointer(&message)))
			if isDialogMessage == 0 {
				procLockTranslate.Call(uintptr(unsafe.Pointer(&message)))
				procLockDispatch.Call(uintptr(unsafe.Pointer(&message)))
			}
		} else {
			procLockWaitMessage.Call()
		}
	}
	if !lockPromptState.accepted {
		return nil, false
	}
	runes := utf16.Decode(lockPromptState.text)
	result := make([]byte, 0, len(runes)*2)
	for _, r := range runes {
		result = utf8.AppendRune(result, r)
	}
	for i := range runes {
		runes[i] = 0
	}
	for i := range lockPromptState.text {
		lockPromptState.text[i] = 0
	}
	lockPromptState.text = nil
	return result, true
}

func nativeTextPrompt(owner uintptr, title, message string) ([]byte, bool) {
	return nativeCredentialPrompt(owner, title, message, false)
}

func nativeAskChoice(owner uintptr, title, message, yesLabel, noLabel string) int {
	message += "\n\nYes: " + yesLabel + "\nNo: " + noLabel
	return nativeMessageChoice(owner, title, message, lockMBYesNoCancel|lockMBIconWarn)
}

func nativeMessageChoice(owner uintptr, title, message string, flags uint32) int {
	titlePtr, messagePtr := syscall.StringToUTF16Ptr(title), syscall.StringToUTF16Ptr(message)
	result, _, _ := procLockMessageBox.Call(owner, uintptr(unsafe.Pointer(messagePtr)), uintptr(unsafe.Pointer(titlePtr)), uintptr(flags|lockMBSetForeground|lockMBTopmost))
	switch result {
	case lockIDYes:
		return 1
	case lockIDNo:
		return 2
	default:
		return 0
	}
}

func nativeInform(owner uintptr, title, message string) {
	titlePtr, messagePtr := syscall.StringToUTF16Ptr(title), syscall.StringToUTF16Ptr(message)
	procLockMessageBox.Call(owner, uintptr(unsafe.Pointer(messagePtr)), uintptr(unsafe.Pointer(titlePtr)), uintptr(lockMBOK|lockMBIconInfo|lockMBSetForeground|lockMBTopmost))
}

func nativeShowRecoveryCode(owner uintptr, code string) bool {
	message := "Save this one-time recovery code somewhere private. It is shown only once. Select Yes only after saving it.\n\n" + code
	return nativeMessageChoice(owner, "Save recovery code", message, lockMBYesNoCancel|lockMBIconInfo) == 1
}

func nativeSetMainWindowVisible(owner uintptr, visible bool) {
	if owner == 0 {
		return
	}
	if visible {
		title := syscall.StringToUTF16Ptr(windowTitle)
		procLockSetWindowText.Call(owner, uintptr(unsafe.Pointer(title)))
		procLockShowWindow.Call(owner, lockSWRestore)
		procSetFgWindow.Call(owner)
	} else {
		title := syscall.StringToUTF16Ptr(windowTitle + " (Locked)")
		procLockSetWindowText.Call(owner, uintptr(unsafe.Pointer(title)))
		procLockShowWindow.Call(owner, lockSWHide)
	}
}

func nativeWindowMinimized(owner uintptr) bool {
	if owner == 0 {
		return false
	}
	value, _, _ := procIsIconic.Call(owner)
	return value != 0
}

func nativeIdleFor(owner uintptr) time.Duration {
	_ = owner
	info := nativeLastInputInfo{Size: uint32(unsafe.Sizeof(nativeLastInputInfo{}))}
	ok, _, _ := procLockLastInput.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return 0
	}
	now, _, _ := procLockGetTickCount.Call()
	return time.Duration(uint32(now)-info.Time) * time.Millisecond
}
