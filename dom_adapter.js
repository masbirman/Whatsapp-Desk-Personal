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
	messageWrapper: [
		'[data-testid="msg-container"]',
		'div[data-id]',
		'.message-in',
		'.message-out'
	],
	statusViewer: [
		'[data-testid*="status-viewer"]',
		'[data-animate-status-v3-modal="true"]',
		'[data-testid="status-v3"]',
		'[role="dialog"][aria-label*="Status" i]',
		'[role="dialog"][aria-label*="status" i][data-testid*="status"]'
	],
	statusViewerMedia: [
		'img[src^="blob:"]', 'video[src^="blob:"]',
		'img', 'video'
	],
	conversationTitle: [
		'#main header [data-testid*="conversation-info-header-chat-title"]',
		'#main header span[title]'
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

	// --- Local identity (M5-01) ------------------------------------------------
	// Identity keys are opaque, adapter-issued strings. A stable WhatsApp
	// attribute (data-id) yields high confidence; a visible-title fallback is
	// explicitly medium and can be invalidated by a rename. The raw attribute
	// is never returned or stored: only the hashed key leaves the adapter.
	var ADAPTER_VERSION = 2;

	function fnv1aHex(input, seed) {
		var hash = seed >>> 0;
		for (var i = 0; i < input.length; i++) {
			hash ^= input.charCodeAt(i);
			hash = (hash + ((hash << 1) >>> 0) + ((hash << 4) >>> 0) + ((hash << 7) >>> 0) + ((hash << 8) >>> 0) + ((hash << 24) >>> 0)) >>> 0;
		}
		return ('00000000' + hash.toString(16)).slice(-8);
	}

	function identityKey(kind, raw) {
		return kind + ':' + fnv1aHex(raw, 0x811c9dc5) + fnv1aHex(raw, 0x01000193);
	}

	function visibleTitle(node) {
		if (!node || typeof node.querySelectorAll !== 'function') return '';
		var candidates = resolveAll('privacyChatNames', node);
		var el = candidates.nodes.length ? candidates.nodes[0] : null;
		var title = el ? (el.getAttribute('title') || el.textContent || '') : '';
		title = String(title).replace(/\s+/g, ' ').trim();
		return title.slice(0, 128);
	}

	function dataIdWithin(node) {
		var current = node && node.nodeType === 1 ? node : null;
		while (current) {
			var value = current.getAttribute && current.getAttribute('data-id');
			if (value) return String(value);
			current = current.parentElement;
		}
		return '';
	}

	// Identity result contract: {status, key, kind, confidence, version}.
	// status: found | ambiguous | missing. Ambiguous or missing inputs never
	// produce a key, so callers cannot persist a wrong-chat reference.
	function identityResult(status, key, kind, confidence) {
		return {
			status: status,
			key: key || '',
			kind: kind || '',
			confidence: confidence || '',
			version: ADAPTER_VERSION
		};
	}

	function chatRowIdentity(row) {
		var raw = dataIdWithin(row);
		if (raw) return identityResult('found', identityKey('chat', 'data-id:' + raw), 'data-id', 'high');
		var title = visibleTitle(row);
		if (title) return identityResult('found', identityKey('chat', 'title:' + title), 'title-fallback', 'medium');
		return identityResult('missing', '', '', '');
	}

	function chatRowByKey(key) {
		if (!key || typeof key !== 'string') return identityResult('missing', '', '', '');
		var rows = resolveAll('chatRow').nodes;
		var matches = [];
		for (var i = 0; i < rows.length; i++) {
			var identity = chatRowIdentity(rows[i]);
			if (identity.status === 'found' && identity.key === key) matches.push(rows[i]);
		}
		if (!matches.length) return identityResult('missing', '', '', '');
		if (matches.length > 1) return identityResult('ambiguous', '', '', '');
		return identityResult('found', key, chatRowIdentity(matches[0]).kind, chatRowIdentity(matches[0]).confidence);
	}

	function messageIdentity(node) {
		var wrapper = closest(node, 'messageWrapper');
		var target = wrapper.status === 'found' ? wrapper.node : node;
		var raw = dataIdWithin(target);
		if (raw) return identityResult('found', identityKey('msg', 'data-id:' + raw), 'data-id', 'high');
		return identityResult('missing', '', '', '');
	}

	function activeChatIdentity() {
		var root = resolveFirst('conversationRoot');
		if (!root.node) return identityResult('missing', '', '', '');
		var titles = resolveAll('conversationTitle', root.node).nodes;
		var title = '';
		for (var i = 0; i < titles.length; i++) {
			title = (titles[i].getAttribute('title') || titles[i].textContent || '').replace(/\s+/g, ' ').trim();
			if (title) break;
		}
		if (!title) return identityResult('missing', '', '', '');
		return {
			status: 'found',
			key: identityKey('chat', 'title:' + title.slice(0, 128)),
			kind: 'title-fallback',
			confidence: 'medium',
			label: title.slice(0, 128),
			version: ADAPTER_VERSION
		};
	}

	function messageNodeByKey(key) {
		if (!key || typeof key !== 'string') return identityResult('missing', '', '', '');
		var root = resolveFirst('conversationRoot');
		if (!root.node) return identityResult('missing', '', '', '');
		var containers = resolveCandidatesInDOMOrder('messageWrapper', root.node);
		var matches = [];
		for (var i = 0; i < containers.length; i++) {
			var identity = messageIdentity(containers[i].node);
			if (identity.status === 'found' && identity.key === key) matches.push(containers[i].node);
		}
		if (!matches.length) return identityResult('missing', '', '', '');
		if (matches.length > 1) return identityResult('ambiguous', '', '', '');
		return identityResult('found', key, 'data-id', 'high');
	}

	// Identity of the last identifiable message in the open conversation:
	// deterministic (last in DOM order with a resolvable data-id) and never
	// guesses when no message carries an identity.
	function latestMessageIdentity() {
		var root = resolveFirst('conversationRoot');
		if (!root.node) return identityResult('missing', '', '', '');
		var containers = resolveCandidatesInDOMOrder('messageWrapper', root.node);
		for (var i = containers.length - 1; i >= 0; i--) {
			var identity = messageIdentity(containers[i].node);
			if (identity.status === 'found') return identity;
		}
		return identityResult('missing', '', '', '');
	}

	// Story/status viewer detection (M6-01). Only the viewer the user has
	// open is inspected; there is deliberately no status list discovery and
	// no background scanning. An explicit unsupported result lets callers
	// disable their actions instead of guessing.
	function openStoryViewer() {
		var viewer = resolveOne('statusViewer');
		if (viewer.status !== 'found' || !viewer.node) {
			return { status: viewer.status === 'ambiguous' ? 'ambiguous' : 'missing', mediaKind: '', version: ADAPTER_VERSION };
		}
		var media = resolveAll('statusViewerMedia', viewer.node).nodes;
		for (var i = 0; i < media.length; i++) {
			var kind = media[i].tagName === 'VIDEO' ? 'video' : (media[i].tagName === 'IMG' ? 'image' : '');
			if (kind) {
				return { status: 'found', mediaKind: kind, node: media[i], version: ADAPTER_VERSION };
			}
		}
		return { status: 'found', mediaKind: 'unknown', node: null, version: ADAPTER_VERSION };
	}

	global.waDOM = Object.freeze({
		version: ADAPTER_VERSION,
		selectors: selectors,
		resolveAll: resolveAll,
		resolveOne: resolveOne,
		resolveFirst: resolveFirst,
		resolveCandidates: resolveCandidates,
		resolveCandidatesInDOMOrder: resolveCandidatesInDOMOrder,
		resolveFirstInDOMOrder: resolveFirstInDOMOrder,
		closest: closest,
		chatRowIdentity: chatRowIdentity,
		chatRowByKey: chatRowByKey,
		messageIdentity: messageIdentity,
		activeChatIdentity: activeChatIdentity,
		latestMessageIdentity: latestMessageIdentity,
		messageNodeByKey: messageNodeByKey,
		openStoryViewer: openStoryViewer
	});
})(window, document);
