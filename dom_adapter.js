// Central compatibility layer for selectors that depend on WhatsApp Web's DOM.
// Keep semantic and accessible selectors first; legacy selectors are bounded fallbacks.
(function installWhatsAppDOMAdapter(global, doc) {
	'use strict';

	var selectorRegistry = Object.freeze({
		chatListRoot: [
			'#side',
			'#pane-side',
			'[data-testid="chat-list"]',
			'div[aria-label="Chat list"]'
		],
		chatRow: [
			'[data-testid="cell-frame-container"]',
			'[role="row"]',
			'[role="listitem"]',
			'div[tabindex="-1"]',
			'div._ak8l'
		],
		chatDropFallback: ['[data-testid="chat-list"]'],
		conversationRoot: [
			'#main',
			'[data-testid="conversation-panel"]'
		],
		messageContainer: [
			'[data-testid*="msg-container"]',
			'.message-in',
			'.message-out',
			'[role="row"]',
			'div[data-id]'
		],
		mediaViewer: [
			'[data-testid="media-viewer"]',
			'[data-animate-media-viewer="true"]'
		],
		mediaViewerCloseControl: [
			'button[data-testid="x-viewer"]',
			'[data-testid="x-viewer"]',
			'[data-icon="x-viewer"]',
			'[data-icon="x"]',
			'[data-icon="back"]',
			'button[aria-label*="Close" i]',
			'button[aria-label*="Tutup" i]',
			'[role="button"][aria-label*="Close" i]',
			'[role="button"][aria-label*="Tutup" i]',
			'button[title*="Close" i]',
			'button[title*="Tutup" i]',
			'[data-testid="btn-close"]',
			'[data-testid="media-viewer-close"]'
		],
		mediaViewerDownloadControl: [
			'button[data-testid*="download"]',
			'[role="button"][data-testid*="download"]',
			'button[aria-label*="Download" i]',
			'button[aria-label*="Unduh" i]',
			'[role="button"][aria-label*="Download" i]',
			'[role="button"][aria-label*="Unduh" i]',
			'button[title*="Download" i]',
			'button[title*="Unduh" i]',
			'[data-icon="download"]',
			'[data-icon="download-refreshed"]',
			'[data-icon*="download"]'
		],
		attachButton: [
			'[data-testid="attach-menu-plus"]',
			'[data-testid="conversation-clip"]',
			'[data-testid="clip"]',
			'[data-icon="clip"]',
			'[data-testid="plus"]',
			'[data-icon="plus"]',
			'#main footer [role="button"][aria-label*="Attach" i]',
			'#main footer [role="button"][aria-label*="Lampirkan" i]',
			'#main footer button[aria-label*="Attach" i]',
			'#main footer button[aria-label*="Lampirkan" i]',
			'button[aria-label*="Attach" i]',
			'button[aria-label*="Lampirkan" i]',
			'[role="button"][aria-label*="Attach" i]',
			'[role="button"][aria-label*="Lampirkan" i]',
			'button[title*="Attach" i]',
			'button[title*="Lampirkan" i]'
		],
		attachMediaItem: [
			'li[data-testid*="attach-media"]',
			'li[data-testid*="attach-image"]',
			'li[data-testid*="image"]',
			'[data-testid*="attach-media"]',
			'[data-testid*="attach-image"]',
			'[data-testid="mi-attach-media"]',
			'[data-testid="attach-image"]',
			'[data-icon="attach-image"]',
			'[data-icon="image"]',
			'[aria-label*="Photos & videos" i]',
			'[aria-label*="Foto & video" i]',
			'[aria-label*="Fotos y videos" i]',
			'[aria-label*="Fotos e vídeos" i]',
			'[title*="Photos & videos" i]',
			'[title*="Foto & video" i]'
		],
		fileInput: ['input[type="file"]'],
		savedFileScanPanel: [
			'[role="dialog"]',
			'[data-testid="media-viewer"]'
		],
		savedFileMessageRow: [
			'[role="row"]',
			'[data-testid="cell-frame-outer"]',
			'.message-in',
			'.message-out'
		],
		documentDownloadControl: [
			'a[download]',
			'button[data-testid*="download"]',
			'[role="button"][data-testid*="download"]',
			'button[aria-label*="Unduh"]',
			'button[aria-label*="Download"]',
			'[role="button"][aria-label*="Unduh"]',
			'[role="button"][aria-label*="Download"]',
			'[data-icon="download"]',
			'[data-icon="download-refreshed"]'
		],
		stickerInputContainer: [
			'[data-testid*="sticker"]',
			'[aria-label*="sticker" i]',
			'[aria-label*="stiker" i]'
		],
		settingsHeader: [
			'#side header',
			'header'
		],
		sideHeader: ['#side header'],
		conversationHeader: [
			'#main header',
			'[data-testid="conversation-panel"] header'
		],
		privacyChatNames: [
			'[data-testid="cell-frame-title"]',
			'[data-testid*="cell-frame-title"]',
			'[role="row"] [title]',
			'[role="listitem"] [title]'
		],
		privacyGroupIndicator: [
			'[data-icon="default-group"]',
			'[data-testid*="group"]',
			'[aria-label*="group" i]',
			'[aria-label*="grup" i]'
		],
		privacyChatPreviews: [
			'[data-testid="last-msg"]',
			'[data-testid*="last-msg"]',
			'[data-testid*="cell-frame-secondary"]'
		],
		privacyTimestamps: [
			'time',
			'[data-testid*="timestamp"]',
			'[data-testid*="cell-frame-time"]',
			'span[aria-label*="AM"]',
			'span[aria-label*="PM"]'
		],
		privacyUnreadCounts: [
			'[data-testid*="unread-count"]',
			'[data-testid*="unread"]',
			'[aria-label*="unread" i]',
			'[aria-label*="belum dibaca" i]'
		],
		privacyAvatars: [
			'img', 'image', 'svg[viewBox="0 0 49 49"]',
			'[data-testid="default-user"]', '[data-icon="default-user"]',
			'[data-icon="default-group"]'
		],
		privacyMessageAvatars: [
			'[data-testid*="msg-container"] ._ak8h',
			'.message-in ._ak8h', '.message-out ._ak8h',
			'[data-testid*="msg-container"] [data-testid="default-user"]'
		],
		privacyMessageText: [
			'[data-testid*="msg-container"] span:not([data-wa-time])',
			'.message-in span:not([data-wa-time])',
			'.message-out span:not([data-wa-time])'
		],
		privacyImages: [
			'[data-testid*="msg-container"] img:not([data-emoji])',
			'.message-in img:not([data-emoji])', '.message-out img:not([data-emoji])'
		],
		privacyVideos: [
			'[data-testid*="msg-container"] video', '.message-in video', '.message-out video'
		],
		privacyStickers: [
			'[data-testid*="sticker"] img', '[data-testid*="sticker"] canvas',
			'[aria-label*="sticker" i] img', '[aria-label*="stiker" i] img'
		],
		privacyQuotedContent: [
			'[data-testid*="quoted"]', '[data-testid*="reply-context"]',
			'[data-testid*="quoted-message"]', '[data-testid*="quoted-message"] span'
		],
		privacyVoiceNoteDetails: [
			'[data-testid*="audio"]', '[data-testid*="voice"]',
			'[aria-label*="voice message" i]', '[aria-label*="pesan suara" i]'
		],
		privacyHeaderNames: [
			'#main header [data-testid*="conversation-info-header-chat-title"]',
			'#main header span[title]'
		],
		privacyHeaderAvatars: [
			'#main header img', '#main header image',
			'#main header [data-testid="default-user"]', '#main header [data-icon="default-user"]'
		],
		privacyHeaderSubtitles: [
			'#main header [data-testid*="conversation-info-header-chat-subtitle"]',
			'#main header span[title]'
		],
		privacyViewerMedia: [
			'[data-testid="media-viewer"] img', '[data-testid="media-viewer"] video',
			'[data-animate-media-viewer="true"] img', '[data-animate-media-viewer="true"] video'
		],
		pageTitle: ['title']
	});

	function selectorsFor(key) {
		return Object.prototype.hasOwnProperty.call(selectorRegistry, key) ? selectorRegistry[key] : null;
	}

	function queryAll(root, selector) {
		if (!root || typeof root.querySelectorAll !== 'function') return [];
		try {
			return Array.prototype.slice.call(root.querySelectorAll(selector));
		} catch (e) {
			return [];
		}
	}

	function result(key, status, selector, nodes) {
		var list = nodes || [];
		return {
			key: key,
			status: status,
			selector: selector || '',
			node: status === 'found' ? (list[0] || null) : null,
			nodes: list,
			version: 1
		};
	}

	function resolveAll(key, root) {
		var selectors = selectorsFor(key);
		if (!selectors) return result(key, 'missing', '', []);
		var scope = root || doc;
		for (var i = 0; i < selectors.length; i++) {
			var found = queryAll(scope, selectors[i]);
			if (found.length) return result(key, 'found', selectors[i], found);
		}
		return result(key, 'missing', '', []);
	}

	// Return candidates in selector-priority order, optionally trying a fallback
	// root for selectors that did not match the preferred scope. Callers that act
	// on identity-sensitive targets should use resolveOne, which fails closed
	// when its preferred selector is ambiguous.
	function resolveCandidates(key, root, fallbackRoot) {
		var selectors = selectorsFor(key);
		if (!selectors) return [];
		var scope = root || doc;
		var fallbackScope = fallbackRoot || null;
		var seen = [];
		var candidates = [];
		for (var i = 0; i < selectors.length; i++) {
			var found = queryAll(scope, selectors[i]);
			if (!found.length && fallbackScope && fallbackScope !== scope) {
				found = queryAll(fallbackScope, selectors[i]);
			}
			for (var j = 0; j < found.length; j++) {
				if (seen.indexOf(found[j]) !== -1) continue;
				seen.push(found[j]);
				candidates.push({ node: found[j], selector: selectors[i], priority: i });
			}
		}
		return candidates;
	}

	// Preserve querySelectorAll(commaSeparatedSelectors) DOM ordering for legacy
	// callers that score equally ranked visible controls by their screen position.
	function resolveCandidatesInDOMOrder(key, root) {
		var selectors = selectorsFor(key);
		if (!selectors) return [];
		var scope = root || doc;
		var found = queryAll(scope, selectors.join(', '));
		var candidates = [];
		for (var i = 0; i < found.length; i++) {
			var matchedSelector = '';
			if (typeof found[i].matches === 'function') {
				for (var j = 0; j < selectors.length; j++) {
					try {
						if (found[i].matches(selectors[j])) {
							matchedSelector = selectors[j];
							break;
						}
					} catch (e) {}
				}
			}
			candidates.push({ node: found[i], selector: matchedSelector, priority: -1 });
		}
		return candidates;
	}

	function resolveFirstInDOMOrder(key, root) {
		var candidates = resolveCandidatesInDOMOrder(key, root);
		if (!candidates.length) return result(key, 'missing', '', []);
		var nodes = candidates.map(function(candidate) { return candidate.node; });
		var resolved = result(key, candidates.length > 1 ? 'ambiguous' : 'found', candidates[0].selector, nodes);
		if (resolved.status === 'ambiguous') resolved.node = nodes[0];
		return resolved;
	}

	function resolveOne(key, root) {
		var found = resolveAll(key, root);
		if (found.nodes.length === 0) return found;
		if (found.nodes.length > 1) return result(key, 'ambiguous', found.selector, found.nodes);
		return found;
	}

	// Explicitly choose the first DOM result only for legacy UI affordances whose
	// existing behavior is first-match selection. The returned status still
	// discloses ambiguity so callers can migrate to a stricter choice later.
	function resolveFirst(key, root) {
		var found = resolveAll(key, root);
		if (found.nodes.length > 1) {
			found.status = 'ambiguous';
			found.node = found.nodes[0];
		}
		return found;
	}

	function closest(node, key, stopAt) {
		var selectors = selectorsFor(key);
		if (!selectors) return result(key, 'missing', '', []);
		var current = node && node.nodeType === 1 ? node : null;
		while (current && current !== stopAt) {
			if (typeof current.matches === 'function') {
				for (var i = 0; i < selectors.length; i++) {
					try {
						if (current.matches(selectors[i])) return result(key, 'found', selectors[i], [current]);
					} catch (e) {}
				}
			}
			current = current.parentElement;
		}
		return result(key, 'missing', '', []);
	}

	function selectors(key) {
		var found = selectorsFor(key);
		return found ? found.slice() : [];
	}

	global.waDOM = Object.freeze({
		version: 1,
		selectors: selectors,
		resolveAll: resolveAll,
		resolveOne: resolveOne,
		resolveFirst: resolveFirst,
		resolveCandidates: resolveCandidates,
		resolveCandidatesInDOMOrder: resolveCandidatesInDOMOrder,
		resolveFirstInDOMOrder: resolveFirstInDOMOrder,
		closest: closest
	});
})(window, document);
