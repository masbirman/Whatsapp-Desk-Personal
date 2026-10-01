package main

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxBridgeURLBytes          = 4096
	maxBridgePathBytes         = 4096
	maxBridgeFilenameBytes     = 1024
	maxBridgeNotificationTitle = 512
	maxBridgeNotificationBody  = 8192
	maxBridgeDimension         = 16384
)

type ExternalLinkRequest struct {
	URL string
}

type ThemeChangeRequest struct {
	Theme string
}

type SpellCheckLanguageRequest struct {
	Language string
}

type SaveFileRequest struct {
	Filename string
	DataURI  string
}

type OpenLocalFileRequest struct {
	Path string
}

type NativeNotificationRequest struct {
	Title string
	Body  string
}

// NotificationEvent is the bounded, page-proposed notification event. It is
// only a proposal: the native policy decides what (if anything) is shown.
type NotificationEvent struct {
	Title    string
	Body     string
	Tag      string
	ChatType string
	Focused  bool
}

type WindowSizeRequest struct {
	Width  int
	Height int
}

type UnreadBadgeRequest struct {
	Value string
}

func newExternalLinkRequest(raw string) (ExternalLinkRequest, error) {
	if len(raw) == 0 || len(raw) > maxBridgeURLBytes || hasControlCharacter(raw) {
		return ExternalLinkRequest{}, errors.New("invalid external link")
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" {
		return ExternalLinkRequest{}, errors.New("external link must be an absolute HTTP(S) URL")
	}
	if _, err := url.Parse(parsed.String()); err != nil {
		return ExternalLinkRequest{}, errors.New("invalid external link")
	}
	return ExternalLinkRequest{URL: parsed.String()}, nil
}

func newThemeChangeRequest(theme string) (ThemeChangeRequest, error) {
	switch theme {
	case "dark", "light", "system":
		return ThemeChangeRequest{Theme: theme}, nil
	default:
		return ThemeChangeRequest{}, errors.New("unsupported theme")
	}
}

func newSpellCheckLanguageRequest(language string) (SpellCheckLanguageRequest, error) {
	if len(language) == 0 || len(language) > 64 || strings.TrimSpace(language) != language {
		return SpellCheckLanguageRequest{}, errors.New("invalid spell-check language")
	}
	if language == "auto" {
		return SpellCheckLanguageRequest{Language: language}, nil
	}
	segmentLength := 0
	for _, r := range language {
		if r == '-' {
			if segmentLength == 0 || segmentLength > 8 {
				return SpellCheckLanguageRequest{}, errors.New("invalid spell-check language")
			}
			segmentLength = 0
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return SpellCheckLanguageRequest{}, errors.New("invalid spell-check language")
		}
		segmentLength++
	}
	if segmentLength == 0 || segmentLength > 8 {
		return SpellCheckLanguageRequest{}, errors.New("invalid spell-check language")
	}
	return SpellCheckLanguageRequest{Language: language}, nil
}

func newSaveFileRequest(filename, dataURI string) (SaveFileRequest, error) {
	if len(filename) == 0 || len(filename) > maxBridgeFilenameBytes || hasControlCharacter(filename) {
		return SaveFileRequest{}, errors.New("invalid filename")
	}
	maxEncodedBytes := maxAttachmentBytes/3*4 + 4096
	if len(dataURI) == 0 || int64(len(dataURI)) > maxEncodedBytes {
		return SaveFileRequest{}, errors.New("file payload exceeds bridge limit")
	}
	return SaveFileRequest{Filename: filename, DataURI: dataURI}, nil
}

func newOpenLocalFileRequest(path string) (OpenLocalFileRequest, error) {
	if len(path) == 0 || len(path) > maxBridgePathBytes || hasControlCharacter(path) {
		return OpenLocalFileRequest{}, errors.New("invalid file path")
	}
	return OpenLocalFileRequest{Path: path}, nil
}

func newNativeNotificationRequest(title, body string) (NativeNotificationRequest, error) {
	if utf8.RuneCountInString(title) > maxBridgeNotificationTitle ||
		utf8.RuneCountInString(body) > maxBridgeNotificationBody ||
		hasControlCharacter(title) {
		return NativeNotificationRequest{}, errors.New("notification exceeds bridge limits")
	}
	return NativeNotificationRequest{Title: title, Body: body}, nil
}

func newNotificationEvent(title, body, tag, chatType string, focused bool) (NotificationEvent, error) {
	if utf8.RuneCountInString(title) > maxBridgeNotificationTitle ||
		utf8.RuneCountInString(body) > maxBridgeNotificationBody ||
		len(tag) > maxNotificationTagBytes ||
		hasControlCharacter(title) || hasControlCharacter(body) || hasControlCharacter(tag) {
		return NotificationEvent{}, errors.New("notification event exceeds bridge limits")
	}
	if !utf8.ValidString(tag) {
		return NotificationEvent{}, errors.New("invalid notification tag")
	}
	switch NotificationChatType(chatType) {
	case ChatTypeUnknown, ChatTypePrivate, ChatTypeGroup:
	default:
		return NotificationEvent{}, errors.New("unsupported notification chat type")
	}
	return NotificationEvent{Title: title, Body: body, Tag: tag, ChatType: chatType, Focused: focused}, nil
}

func newWindowSizeRequest(width, height int) (WindowSizeRequest, error) {
	if width < 450 || height < 320 || width > maxBridgeDimension || height > maxBridgeDimension {
		return WindowSizeRequest{}, errors.New("window dimensions are outside supported limits")
	}
	return WindowSizeRequest{Width: width, Height: height}, nil
}

func newUnreadBadgeRequest(value string) (UnreadBadgeRequest, error) {
	if len(value) > 32 || hasControlCharacter(value) {
		return UnreadBadgeRequest{}, errors.New("invalid unread badge")
	}
	return UnreadBadgeRequest{Value: value}, nil
}

func hasControlCharacter(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) }) >= 0
}

func saveDownloadedFileFromBridge(filename, dataURI string) (string, error) {
	request, err := newSaveFileRequest(filename, dataURI)
	if err != nil {
		return "", err
	}
	return saveDownloadedFile(request.Filename, request.DataURI)
}

func previewDocumentFromBridge(filename, dataURI string) (string, error) {
	request, err := newSaveFileRequest(filename, dataURI)
	if err != nil {
		return "", err
	}
	return previewDocument(request.Filename, request.DataURI)
}

func openFileFromBridge(path string) bool {
	request, err := newOpenLocalFileRequest(path)
	return err == nil && openFileInDefaultApp(request.Path)
}
