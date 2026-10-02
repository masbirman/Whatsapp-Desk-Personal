package main

import (
	"runtime"
	"strings"
)

const (
	windowWidth  = 1200
	windowHeight = 800
)

func getInitScript(ua string) string {
	clientPlatform := "macOS"
	clientPlatformVersion := "15.0.0"
	clientArch := "arm"
	if runtime.GOOS == "windows" {
		clientPlatform = "Windows"
		clientPlatformVersion = "10.0.0"
		clientArch = "x86"
	} else if runtime.GOOS == "linux" {
		clientPlatform = "Linux"
		clientPlatformVersion = "6.8.0"
		clientArch = "x86"
	}

	script := `
		// --- Safe storage -------------------------------------------------
		// localStorage throws (SecurityError) instead of returning null when
		// the engine denies storage: WebView2 does this in InPrivate mode and
		// when the profile directory is read-only. Unguarded calls used to
		// abort the whole injected script, which took the Settings control and
		// every keyboard shortcut down with it. All reads/writes go through
		// here so a denied store degrades to "preference not persisted".
		var waMemoryStore = {};
		function storageGet(key) {
			try {
				var v = localStorage.getItem(key);
				if (v !== null) return v;
			} catch (e) {
				waNoteRecoverable('storage-get', e);
			}
			return Object.prototype.hasOwnProperty.call(waMemoryStore, key) ? waMemoryStore[key] : null;
		}
		function storageSet(key, value) {
			waMemoryStore[key] = String(value);
			try {
				localStorage.setItem(key, String(value));
			} catch (e) {
				waNoteRecoverable('storage-set', e);
			}
		}
		function storageRemove(key) {
			delete waMemoryStore[key];
			try {
				localStorage.removeItem(key);
			} catch (e) {
				waNoteRecoverable('storage-remove', e);
			}
		}

		// --- Recoverable-failure log --------------------------------------
		// Non-fatal problems are recorded instead of thrown so one degraded
		// feature never disables the rest of the injected script. Bounded, and
		// surfaced by the in-app diagnostics panel.
		var waRecoverable = [];
		function waNoteRecoverable(where, err) {
			try {
				waRecoverable.push(where + ': ' + String((err && err.message) || err));
				if (waRecoverable.length > 25) waRecoverable.shift();
			} catch (e) {}
		}
		window.__waRecoverable = function() { return waRecoverable.slice(); };

		// Runs a module so that a failure inside it cannot stop later modules.
		// Each IIFE below is independent; without this an early throw (a WebView2
		// API difference, a denied storage read) removes every enhancement
		// defined after it.
		function waRunModule(name, fn) {
			try {
				return fn();
			} catch (e) {
				waNoteRecoverable(name, e);
				return undefined;
			}
		}

		// __WA_DOM_ADAPTER__

		// Go-side platform constant — more reliable than navigator.platform which is
		// deprecated in Chrome 93+ and may return "" in newer WebView2 builds.
		var __WA_GOOS = '` + runtime.GOOS + `';

	try {
		// UserAgent and platform override to Google Chrome
		Object.defineProperty(navigator, 'userAgent', {
			get: () => '` + ua + `'
		});
		Object.defineProperty(navigator, 'appVersion', {
			get: () => '` + ua + `'
		});
		Object.defineProperty(navigator, 'vendor', {
			get: () => 'Google Inc.'
		});

		// Emulate window.chrome
		if (!window.chrome) {
			window.chrome = {
				app: { isInstalled: false },
				runtime: {}
			};
		}

		// Remove Safari-specific markers
		try {
			delete window.safari;
		} catch (e) {}

		// NOTE (v1.5.9): a <meta> Content-Security-Policy allowlist was tried in
		// v1.5.8 and REVERTED — WhatsApp Web loads its boot bundles from Meta
		// CDN hosts outside any maintainable allowlist, so the policy blocked
		// boot and left the app stuck on the splash screen. Do not re-add a
		// meta CSP without a report-only phase first.

		// WhatsApp's virtualized lists can emit hundreds of DOM mutations while the
		// user scrolls. Our enhancements are non-critical during that gesture, so
		// defer them briefly instead of competing with WebKit's renderer. This is
		// deliberately a shared gate: observers keep their correctness but never
		// create a second rendering workload during fast scrolling.
		var waBackgroundWorkBusyUntil = 0;
		function markBackgroundWorkBusy() {
			waBackgroundWorkBusyUntil = Date.now() + 350;
		}
		window.addEventListener('scroll', markBackgroundWorkBusy, { passive: true, capture: true });
		window.addEventListener('wheel', markBackgroundWorkBusy, { passive: true, capture: true });
		window.addEventListener('touchmove', markBackgroundWorkBusy, { passive: true, capture: true });
		function shouldPauseBackgroundWork() {
			return document.hidden === true || Date.now() < waBackgroundWorkBusyUntil;
		}

		// Emulate navigator.userAgentData (User-Agent Client Hints)
		if (!navigator.userAgentData) {
			Object.defineProperty(navigator, 'userAgentData', {
				get: () => ({
					brands: [
						{ brand: 'Not(A:Brand', version: '99' },
						{ brand: 'Google Chrome', version: '133' },
						{ brand: 'Chromium', version: '133' }
					],
					mobile: false,
					platform: '` + clientPlatform + `',
					getHighEntropyValues: function() {
						return Promise.resolve({
							architecture: '` + clientArch + `',
							bitness: '64',
							brands: [
								{ brand: 'Not(A:Brand', version: '99' },
								{ brand: 'Google Chrome', version: '133' },
								{ brand: 'Chromium', version: '133' }
							],
							fullVersionList: [
								{ brand: 'Not(A:Brand', version: '99.0.0.0' },
								{ brand: 'Google Chrome', version: '133.0.0.0' },
								{ brand: 'Chromium', version: '133.0.0.0' }
							],
							mobile: false,
							model: '',
							platform: '` + clientPlatform + `',
							platformVersion: '` + clientPlatformVersion + `',
							uaFullVersion: '133.0.0.0'
						});
					}
				})
			});
		}

		// Keep WKWebView's real PDF capability untouched. Advertising Chrome's
		// PDF plugin makes WhatsApp open a viewer that WKWebView cannot render.

		// Native Notification Polyfill & ServiceWorker Notification Interceptor
		waRunModule('notifications', function() {
			var notificationsEnabled = true;
			var notificationsStateReady = false;
			window.isNotificationsEnabled = function() {
				return notificationsStateReady && notificationsEnabled;
			};
			window.setNotificationsEnabled = function(enabled) {
				enabled = !!enabled;
				if (!window.setNotificationsEnabledNative) {
					notificationsEnabled = enabled;
					notificationsStateReady = true;
					return Promise.resolve(notificationsEnabled);
				}
				return Promise.resolve(window.setNotificationsEnabledNative(enabled)).then(function(saved) {
					notificationsEnabled = !!saved;
					notificationsStateReady = true;
					return notificationsEnabled;
				});
			};
			window.refreshNotificationsEnabled = function() {
				if (!window.getNotificationsEnabledNative) {
					notificationsStateReady = true;
					return Promise.resolve(notificationsEnabled);
				}
				return Promise.resolve(window.getNotificationsEnabledNative()).then(function(saved) {
					notificationsEnabled = !!saved;
					notificationsStateReady = true;
					return notificationsEnabled;
				}).catch(function() {
					notificationsStateReady = true;
					return notificationsEnabled;
				});
			};
			window.refreshNotificationsEnabled();

			function dispatchNativeNotification(title, options) {
				options = options || {};
				var body = options.body || '';
				if (notificationsStateReady && notificationsEnabled && window.sendNativeNotification) {
					// The page only proposes an event; Go decides the presentation.
					var focused = !!(document.hasFocus && document.hasFocus());
					var chatType = 'unknown';
					if (options && typeof options['wa-chat-type'] === 'string') {
						chatType = options['wa-chat-type'];
					}
					window.sendNativeNotification(title, body, String(options.tag || ''), chatType, focused);
				}
			}

			window.Notification = function(title, options) {
				options = options || {};
				dispatchNativeNotification(title, options);
				this.title = title;
				this.body = options.body || '';
				this.onclick = null;
				this.onclose = null;
				this.onerror = null;
				this.onshow = null;
			};
			window.Notification.permission = 'granted';
			window.Notification.maxActions = 2;
			window.Notification.requestPermission = function(callback) {
				var p = Promise.resolve('granted');
				if (typeof callback === 'function') {
					callback('granted');
				}
				return p;
			};

			try {
				if (typeof ServiceWorkerRegistration !== 'undefined' && ServiceWorkerRegistration.prototype) {
					ServiceWorkerRegistration.prototype.showNotification = function(title, options) {
						dispatchNativeNotification(title, options);
						return Promise.resolve();
					};
				}
			} catch (e) {}
		});

		// Robust HTML5 Media Autoplay & Inline Playback Support for Status/Stories and Videos
		waRunModule('media-playback', function() {
			if (!window.HTMLMediaElement) return;

			function prepareMedia(el) {
				if (!el || el.__wa_media_ready) return;
				el.__wa_media_ready = true;
				if (el.tagName === 'VIDEO') {
					el.setAttribute('playsinline', '');
					el.setAttribute('webkit-playsinline', '');
					el.setAttribute('x5-playsinline', '');
				}
				if (!el.getAttribute('preload')) {
					el.setAttribute('preload', 'metadata');
				}
			}

			var origPlay = HTMLMediaElement.prototype.play;
			HTMLMediaElement.prototype.play = function() {
				var self = this;
				prepareMedia(self);
				var res = origPlay.apply(this, arguments);
				if (res && typeof res.catch === 'function') {
					return res.catch(function(err) {
						// When WebKit blocks unmuted autoplay, mute the media and retry playback
						if (err && (err.name === 'NotAllowedError' || err.name === 'AbortError')) {
							self.muted = true;
							return origPlay.apply(self);
						}
						return Promise.reject(err);
					});
				}
				return res;
			};

			// Automatically prepare video/audio elements injected into DOM. WhatsApp's
			// virtualized chat list mutates frequently, so queue only newly-added
			// subtrees and process them once per animation frame. Rescanning the entire
			// document on every busy frame makes scrolling unnecessarily expensive.
			if (window.MutationObserver) {
				var mediaScanScheduled = false;
				var pendingMediaRoots = [];
				function queueMediaRoot(node) {
					if (!node || node.nodeType !== 1 || pendingMediaRoots.length >= 24) return;
					try {
						if ((node.matches && node.matches('video, audio')) ||
							(node.querySelector && node.querySelector('video, audio'))) {
							pendingMediaRoots.push(node);
						}
					} catch (e) {}
				}
				function scanForUnpreparedMedia() {
					mediaScanScheduled = false;
					var roots = pendingMediaRoots.splice(0, pendingMediaRoots.length);
					if (shouldPauseBackgroundWork()) return;
					for (var r = 0; r < roots.length; r++) {
						var root = roots[r];
						if (root.matches && root.matches('video, audio')) prepareMedia(root);
						if (!root.querySelectorAll) continue;
						var list = root.querySelectorAll('video, audio');
						for (var l = 0; l < list.length; l++) prepareMedia(list[l]);
					}
				}
				function scheduleMediaScan() {
					if (mediaScanScheduled || pendingMediaRoots.length === 0) return;
					mediaScanScheduled = true;
					requestAnimationFrame(scanForUnpreparedMedia);
				}
				var mediaObserver = new MutationObserver(function(mutations) {
					if (shouldPauseBackgroundWork()) return;
					for (var m = 0; m < mutations.length; m++) {
						var added = mutations[m].addedNodes;
						for (var n = 0; n < added.length; n++) queueMediaRoot(added[n]);
					}
					scheduleMediaScan();
				});
				var targetNode = document.documentElement || document.body || document;
				if (targetNode && targetNode.nodeType) {
					try {
						mediaObserver.observe(targetNode, { childList: true, subtree: true });
						queueMediaRoot(targetNode);
						scheduleMediaScan();
					} catch (e) {}
				} else {
					document.addEventListener('DOMContentLoaded', function() {
						var root = document.body || document.documentElement || document;
						if (root && root.nodeType) {
							try {
								mediaObserver.observe(root, { childList: true, subtree: true });
								queueMediaRoot(root);
								scheduleMediaScan();
							} catch (e) {}
						}
					});
				}
			}
		});

		function isDocumentFileName(name) {
			if (!name) return false;
			var ext = name.toLowerCase();
			return ext.endsWith('.pdf') || ext.endsWith('.doc') || ext.endsWith('.docx') ||
				   ext.endsWith('.xls') || ext.endsWith('.xlsx') || ext.endsWith('.ppt') ||
				   ext.endsWith('.pptx') || ext.endsWith('.txt') || ext.endsWith('.csv') ||
				   ext.endsWith('.rtf');
		}

		function cleanDownloadFilename(name) {
			if (!name) return '';
			try {
				var clean = String(name).split(/[\\/]/).pop().trim();
				clean = clean.replace(/[\u0000-\u001f]/g, '');
				return clean;
			} catch (e) {
				return '';
			}
		}

		// Escape a string for interpolation into innerHTML or an HTML
		// attribute. Filenames and paths here originate from chat content
		// (extractDocumentName) or release metadata, so they must never be
		// concatenated raw: a name like '"><img src=x onerror=...>x.pdf'
		// would otherwise execute in the privileged page context that can
		// reach every native bridge. Valid names render identically.
		function escapeHtml(s) {
			return String(s == null ? '' : s).replace(/[&<>"']/g, function(c) {
				return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
			});
		}

		// XLSX.utils.sheet_to_html escapes cell text but writes the raw value
		// into a data-v attribute, so a cell whose value is '"><img src=x
		// onerror=...>' closes the attribute early and injects live markup --
		// reachable from any spreadsheet sent in a chat. Parse the generated
		// markup inside an inert <template> (its content is a separate document
		// fragment, so images do not load and handlers never fire) and drop
		// every attribute we do not control, leaving cell content as text only.
		function sanitizeSheetHtml(html) {
			var tpl = document.createElement('template');
			tpl.innerHTML = String(html == null ? '' : html);
			// Elements a spreadsheet cell must never be able to introduce. The
			// attribute pass below already strips on* handlers and src/href, but
			// leaving an inert <img>/<iframe> behind would still be a rendering
			// artifact, so remove them outright.
			var banned = tpl.content.querySelectorAll('script,style,img,svg,iframe,frame,object,embed,link,meta,base,form,input,button,textarea,select,audio,video,source,track,math,template');
			for (var b = banned.length - 1; b >= 0; b--) {
				if (banned[b].parentNode) banned[b].parentNode.removeChild(banned[b]);
			}
			var nodes = tpl.content.querySelectorAll('*');
			for (var i = 0; i < nodes.length; i++) {
				var attrs = nodes[i].attributes;
				for (var a = attrs.length - 1; a >= 0; a--) {
					var name = attrs[a].name.toLowerCase();
					if (name === 'id' || name === 'colspan' || name === 'rowspan') continue;
					nodes[i].removeAttribute(attrs[a].name);
				}
			}
			return tpl.innerHTML;
		}

		function isPlaceholderDownloadFilename(name) {
			var clean = cleanDownloadFilename(name).toLowerCase();
			if (!clean) return true;
			var stem = clean.replace(/\.[^.]+$/, '').replace(/\s*\(\d+\)$/, '').trim();
			return ['document', 'download', 'file', 'attachment', 'whatsapp_file', 'whatsapp_media'].indexOf(stem) !== -1;
		}

		function filenameFromContentDisposition(header) {
			if (!header) return '';
			try {
				var encoded = header.match(/filename\*\s*=\s*[^']*''([^;]+)/i);
				if (encoded && encoded[1]) return cleanDownloadFilename(decodeURIComponent(encoded[1].replace(/^\"|\"$/g, '')));
				var plain = header.match(/filename\s*=\s*(?:\"([^\"]+)\"|([^;]+))/i);
				return cleanDownloadFilename(plain ? (plain[1] || plain[2]) : '');
			} catch (e) {
				return '';
			}
		}

		function resolveDownloadFilename(filename, contentDisposition) {
			var candidates = [filenameFromContentDisposition(contentDisposition), lastClickedDocName, filename];
			for (var i = 0; i < candidates.length; i++) {
				var candidate = cleanDownloadFilename(candidates[i]);
				if (candidate && !isPlaceholderDownloadFilename(candidate)) return candidate;
			}
			for (var j = 0; j < candidates.length; j++) {
				var fallback = cleanDownloadFilename(candidates[j]);
				if (fallback) return fallback;
			}
			return 'whatsapp_file';
		}

		// Dismiss WhatsApp Web's internal stuck viewer overlay
		function dismissStuckViewer() {
			var attempts = 0;
			var dismissTimer = setInterval(function() {
				attempts++;
				if (attempts > 30) {
					clearInterval(dismissTimer);
					return;
				}
				var viewer = window.waDOM.resolveFirstInDOMOrder('mediaViewer', document).node;
				if (!viewer) {
					return;
				}
				var closeCandidates = window.waDOM.resolveCandidates('mediaViewerCloseControl', viewer, document);
				var closed = false;
				for (var i = 0; i < closeCandidates.length; i++) {
					var el = closeCandidates[i].node;
					if (el) {
						var btn = (el.closest && el.closest('button, [role="button"]')) || el;
						try {
							btn.click();
							closed = true;
							break;
						} catch (e) {}
					}
				}
				// Dispatch synthetic Escape tagged so our preview modal ignores it
				var escEvt = new KeyboardEvent('keydown', { key: 'Escape', code: 'Escape', keyCode: 27, which: 27, bubbles: true, cancelable: true });
				escEvt._waViewerDismiss = true;
				try {
					viewer.dispatchEvent(escEvt);
					var app = document.getElementById('app');
					if (app) app.dispatchEvent(escEvt);
				} catch (e) {}

				if (closed || attempts > 6) {
					viewer.style.display = 'none';
					clearInterval(dismissTimer);
				}
			}, 80);
		}
		window.dismissStuckViewer = dismissStuckViewer;

		// Track clicked document filenames with robust regex matching
		var lastClickedDocName = '';
		var lastDocumentIntentAt = 0;
		function extractDocumentName(el) {
			if (!el || typeof el.closest !== 'function') return '';
			// NEVER extract document names from inside the media viewer, modal dialogs, or top toolbars
			if (window.waDOM.closest(el, 'mediaViewer').status === 'found' ||
			    el.closest('#wa-doc-modal-overlay') ||
			    el.closest('[role="toolbar"]') ||
			    el.closest('header')) {
				return '';
			}

			// Only search within a chat message container / row / bubble
			var msgContainer = window.waDOM.closest(el, 'messageContainer').node;
			if (!msgContainer) return '';

			var node = el;
			while (node && node !== msgContainer.parentElement && node !== document.body) {
				var title = node.getAttribute && (node.getAttribute('title') || node.getAttribute('aria-label') || '');
				var titleMatch = title && title.match(/([^\n\r<>]{1,180}\.(pdf|docx?|xlsx?|pptx?|txt|csv|rtf))\b/i);
				if (titleMatch && titleMatch[1]) return titleMatch[1].trim();

				// Check text only on leaf-ish nodes to prevent matching unrelated long container text
				if (!node.children || node.children.length < 5) {
					var text = (node.innerText || '').trim();
					if (text.length > 0 && text.length < 250) {
						var textMatch = text.match(/([^\n\r<>]{1,180}\.(pdf|docx?|xlsx?|pptx?|txt|csv|rtf))\b/i);
						if (textMatch && textMatch[1]) return textMatch[1].trim();
					}
				}
				if (node === msgContainer) break;
				node = node.parentElement;
			}
			return '';
		}
		function isRecentPDFIntent() {
			return !!lastClickedDocName && isDocumentFileName(lastClickedDocName) &&
				(Date.now() - lastDocumentIntentAt) < 20000;
		}
		document.addEventListener('click', function(e) {
			var name = extractDocumentName(e.target);
			if (name) {
				lastClickedDocName = name;
				lastDocumentIntentAt = Date.now();
			}
		}, true);

		window.closeDocumentViewerAfterNativePreview = function() {
			// The native PDF window is already closed at this point. Only dismiss
			// WhatsApp's own media viewer if it is still present; never send a
			// global Escape because WhatsApp may interpret it as closing the chat.
				var viewer = window.waDOM.resolveFirst('mediaViewer', document).node;
				var candidates = viewer ? window.waDOM.resolveCandidates('mediaViewerCloseControl', viewer) : [];
				var best = null;
				var bestScore = -1;
				for (var i = 0; i < candidates.length; i++) {
					var raw = candidates[i].node;
				if (raw.closest && raw.closest('#wa-doc-modal-overlay')) continue;
				var control = (raw.closest && raw.closest('button, [role="button"]')) || raw;
				var rect = control.getBoundingClientRect();
				if (rect.width < 8 || rect.height < 8 || rect.bottom <= 0 || rect.right <= 0 ||
					rect.top >= window.innerHeight || rect.left >= window.innerWidth) continue;
				var style = window.getComputedStyle(control);
				if (style.display === 'none' || style.visibility === 'hidden' || Number(style.opacity) === 0) continue;
				var score = 0;
				if (rect.top < window.innerHeight * 0.3) score += 4;
				if (rect.left > window.innerWidth * 0.7) score += 4;
				if (control.closest && control.closest('[role="dialog"], [data-testid*="viewer"], header, [role="toolbar"]')) score += 5;
				if (score > bestScore) { best = control; bestScore = score; }
			}

			if (best && bestScore >= 8) {
				best.click();
			}
			lastDocumentIntentAt = 0;
			lastClickedDocName = '';
		};

		// Handle explicit user clicks on WhatsApp Web's Media Viewer ✕ close button
		// Guarantees immediate exit to chat view even if internal viewer state is desynced
		document.addEventListener('click', function(e) {
			var target = e.target;
			if (!target || typeof target.closest !== 'function') return;
			var viewer = window.waDOM.closest(target, 'mediaViewer').node;
			if (!viewer) return;

			var isCloseBtn = window.waDOM.closest(target, 'mediaViewerCloseControl', viewer).status === 'found';

			if (isCloseBtn) {
				lastDocumentIntentAt = 0;
				lastClickedDocName = '';
				setTimeout(function() {
					var activeViewer = window.waDOM.resolveFirst('mediaViewer', document).node;
					if (activeViewer) {
						var escEvt = new KeyboardEvent('keydown', {
							key: 'Escape',
							code: 'Escape',
							keyCode: 27,
							which: 27,
							bubbles: true,
							cancelable: true
						});
						document.dispatchEvent(escEvt);
						window.dispatchEvent(escEvt);
					}
				}, 60);
			}
		}, false);

		// Intercept external link clicks to open in default browser
		document.addEventListener('click', function(e) {
			var target = e.target;
			while (target && target !== document.body && target.tagName !== 'A') {
				target = target.parentElement;
			}
			if (target && target.tagName === 'A' && target.href) {
				try {
					var url = new URL(target.href);
					if (!url.hostname.endsWith('whatsapp.com') && !url.hostname.endsWith('whatsapp.net') && (url.protocol === 'http:' || url.protocol === 'https:')) {
						e.preventDefault();
						e.stopPropagation();
						if (window.openExternalLink) {
							window.openExternalLink(target.href);
						}
					}
				} catch(err) {}
			}
		}, true);

		// Drag & Drop file upload to chat (stabilized for macOS and Windows)
		var lastUploadAt = 0;
		var lastExplicitDownloadAt = 0;
		function isRecentUpload() {
			return (Date.now() - lastUploadAt) < 6000;
		}
		function isRecentExplicitDownload() {
			return (Date.now() - lastExplicitDownloadAt) < 6000;
		}

		waRunModule('drag-drop-paste', function() {
			var dragCounter = 0;
			var dropInProgress = false;

			function getDropZone() {
				return window.waDOM.resolveFirst('conversationRoot', document).node || window.waDOM.resolveFirst('chatDropFallback', document).node || document.body;
			}

			function isFileDrag(e) {
				var dt = e.dataTransfer;
				if (!dt) return false;
				if (dt.types) {
					for (var i = 0; i < dt.types.length; i++) {
						if (dt.types[i] === 'Files') return true;
					}
				}
				return false;
			}

			function isChatDrop(e) {
				var target = e.target;
				if (target && target.closest && target.closest('#wa-settings-modal, #wa-doc-modal-overlay, #wa-onboarding-overlay, [role="dialog"]')) {
					if (e.stopImmediatePropagation) e.stopImmediatePropagation();
					return false;
				}
				return true;
			}

			function handleDragEnter(e) {
				if (!isFileDrag(e) || !isChatDrop(e)) return;
				dragCounter++;
				e.preventDefault();
				// Do NOT stopPropagation — let WhatsApp's own dragenter handlers also fire
				// so its native drop zone activates (needed for document drops)
				var dz = getDropZone();
				if (dz) dz.classList.add('wa-drag-over');
			}

			function handleDragLeave(e) {
				dragCounter--;
				if (dragCounter <= 0) {
					dragCounter = 0;
					var dz = getDropZone();
					if (dz) dz.classList.remove('wa-drag-over');
				}
			}

			function handleDragOver(e) {
				if (!isFileDrag(e) || !isChatDrop(e)) return;
				e.preventDefault();
				// Do NOT stopPropagation — WhatsApp needs dragover to reach #main
				// for its native drop handler to accept the drop event
				e.dataTransfer.dropEffect = 'copy';
			}

			function isMediaFile(file) {
				if (!file) return false;
				var t = (file.type || '').toLowerCase();
				var n = (file.name || '').toLowerCase();
				if (t.startsWith('image/') || t.startsWith('video/')) return true;
				return /\.(jpe?g|png|gif|webp|bmp|svg|ico|heic|heif|mp4|mov|m4v|3gp|mkv|avi|webm)$/i.test(n);
			}

			function areAllMediaFiles(files) {
				if (!files || !files.length) return false;
				for (var i = 0; i < files.length; i++) {
					if (!isMediaFile(files[i])) return false;
				}
				return true;
			}

			function findAttachButton() {
				var selectors = window.waDOM.selectors('attachButton');
				try { return document.querySelector(selectors.join(', ')); } catch (e) { return null; }
			}

			function findInputInOrNear(el) {
				if (!el) return null;
				var inp = el.querySelector('input[type="file"]');
				if (inp) return inp;
				var container = el.closest('li, [role="menuitem"], [role="button"], [data-testid*="attach"]');
				if (container) {
					inp = container.querySelector('input[type="file"]');
					if (inp) return inp;
				}
				if (el.parentElement) {
					inp = el.parentElement.querySelector('input[type="file"]');
					if (inp) return inp;
				}
				return null;
			}

			function findMediaInput() {
				var candidates = window.waDOM.resolveCandidates('attachMediaItem', document);
				for (var s = 0; s < candidates.length; s++) {
					var el = candidates[s].node;
					if (el) {
						var inp = findInputInOrNear(el);
						if (inp) return inp;
					}
				}

				var allInputs = window.waDOM.resolveAll('fileInput', document).nodes;
				for (var i = 0; i < allInputs.length; i++) {
					var input = allInputs[i];
					if (window.waDOM.closest(input, 'stickerInputContainer').status === 'found') {
						continue;
					}
					var accept = (input.getAttribute('accept') || '').toLowerCase();
					if (accept.indexOf('image/png,image/jpeg,image/webp') !== -1 && accept.indexOf('image/*') === -1) {
						continue;
					}
					if (accept.indexOf('image/*') !== -1 || accept.indexOf('video') !== -1) {
						return input;
					}
				}
				return null;
			}

			// Find a hidden document file input that is pre-rendered in the DOM by WhatsApp Web.
			// WhatsApp pre-renders hidden file inputs even before the attach menu is opened.
			// The document input typically has accept="*" or no accept attribute.
			function findDocumentInput() {
				var allInputs = window.waDOM.resolveAll('fileInput', document).nodes;
				for (var i = 0; i < allInputs.length; i++) {
					var input = allInputs[i];

					// Skip sticker inputs
					if (window.waDOM.closest(input, 'stickerInputContainer').status === 'found') {
						continue;
					}

					var accept = (input.getAttribute('accept') || '').toLowerCase().trim();

					// Skip clearly media-only inputs (image/* or video/* without broad acceptance)
					if (accept === 'image/*' || accept === 'video/*') continue;
					if (accept.indexOf('image/*') !== -1 && accept.indexOf('video') !== -1 &&
					    accept.indexOf('pdf') === -1 && accept.indexOf('application') === -1 && accept !== '*') {
						continue;
					}
					if (accept.indexOf('image/png,image/jpeg,image/webp') !== -1 && accept.indexOf('*') === -1) {
						continue;
					}

					// Document inputs:
					//  - accept="*" or accept="*/*" (accept all)
					//  - accept="" or no accept attribute (no restriction)
					//  - accept contains document/application types
					if (accept === '*' || accept === '*/*' || accept === '' ||
					    accept.indexOf('document') !== -1 || accept.indexOf('application') !== -1 ||
					    accept.indexOf('pdf') !== -1) {
						return input;
					}
				}
				return null;
			}

			function setFilesOnInput(fileInput, files) {
				if (!fileInput || !files || files.length === 0) return false;
				try {
					var dt = new DataTransfer();
					for (var i = 0; i < files.length; i++) dt.items.add(files[i]);
					var setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'files');
					if (setter && setter.set) {
						setter.set.call(fileInput, dt.files);
					} else {
						fileInput.files = dt.files;
					}
					if (!fileInput.files || fileInput.files.length !== files.length) {
						fileInput.files = dt.files;
					}
					fileInput.dispatchEvent(new Event('input', { bubbles: true, composed: true }));
					fileInput.dispatchEvent(new Event('change', { bubbles: true, composed: true }));
					return true;
				} catch (err) {
					return false;
				}
			}

			// Only used for media injection (documents are handled natively by WhatsApp).
			function injectFiles(files, attempt, isMedia) {
				if (isMedia === undefined) isMedia = areAllMediaFiles(files);

				var targetInput = isMedia ? findMediaInput() : findDocumentInput();
				if (targetInput && setFilesOnInput(targetInput, files)) {
					return true;
				}

				if (attempt < 40) {
					setTimeout(function() { injectFiles(files, attempt + 1, isMedia); }, 40);
				}
				return false;
			}

			function clearDragVisualState() {
				dragCounter = 0;
				var dz = getDropZone();
				if (dz) dz.classList.remove('wa-drag-over');
			}

			function handleDrop(e) {
				// Reset the drag visual state on EVERY drop, before any early
				// return. The wa-drag-over class sets pointer-events:none on
				// every element, so a drop that lands on an excluded target
				// (a dialog, the settings modal) or arrives with an empty
				// file list (cloud placeholder files, e.g. OneDrive on
				// Windows) used to leave the class stuck until reload — every
				// click in the app went dead, including selecting a contact
				// from the @mention popup, while typing and Enter kept
				// working.
				clearDragVisualState();
				if (!isFileDrag(e) || !isChatDrop(e) || dropInProgress) return;

				var files = Array.prototype.slice.call((e.dataTransfer && e.dataTransfer.files) || []);
				if (!files || files.length === 0) return;

				var isMedia = areAllMediaFiles(files);

				// Prevent browser navigation (navigating to file:// URL)
				e.preventDefault();
				lastUploadAt = Date.now(); // prevent download interceptor from triggering

				// Do NOT stopImmediatePropagation so WhatsApp's native drop handler
				// on #main / conversation-panel receives the drop event for BOTH
				// media (photos/videos) and documents (PDF, Office, etc.).
				// Fallback: if WhatsApp's native editor has not appeared after a
				// few probes, attempt programmatic injection. A single 400ms
				// check raced the editor mount on slower machines and injected a
				// second batch over the native one, so probe several rounds and
				// only inject when no editor has shown up the whole time.
				var waNativeEditorChecks = 0;
				var waNativeEditorPoll = setInterval(function() {
					waNativeEditorChecks++;
					var modalOpen = document.querySelector(
						'[data-testid="media-editor"], [data-testid="image-editor"], ' +
						'[data-testid="drawer-middle"], [role="dialog"], [data-animate-modal-popup="true"]'
					);
					if (modalOpen || waNativeEditorChecks >= 4) {
						clearInterval(waNativeEditorPoll);
						if (!modalOpen) {
							injectFiles(files, 0, isMedia);
						}
					}
				}, 350);
			}

			// File-picker uploads do not pass through the drag/drop handler above.
			// Mark them as uploads as soon as WhatsApp receives the selected files so
			// the document preview hooks below do not mistake the composer blob for a
			// downloaded attachment.
			function handleFileInputChange(e) {
				var input = e && e.target;
				if (!input || input.tagName !== 'INPUT' || input.type !== 'file') return;
				if (input.files && input.files.length > 0) lastUploadAt = Date.now();
			}

			document.addEventListener('dragenter', handleDragEnter, true);
			document.addEventListener('dragleave', handleDragLeave, true);
			document.addEventListener('dragover', handleDragOver, true);
			document.addEventListener('drop', handleDrop, true);
			// Safety nets: a drag that never produces a matching dragleave
			// (cancelled via Esc, source outside the page, or the window
			// losing focus mid-drag) must not leave the class behind.
			document.addEventListener('dragend', clearDragVisualState, true);
			window.addEventListener('blur', clearDragVisualState);
			document.addEventListener('change', handleFileInputChange, true);
		});

		// Helper: Decode base64 dataURI to Uint8Array
		function base64ToUint8Array(dataUri) {
			try {
				var base64 = dataUri.indexOf(';base64,') !== -1 ? dataUri.split(';base64,')[1] : dataUri;
				var binary = atob(base64);
				var bytes = new Uint8Array(binary.length);
				for (var i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
				return bytes;
			} catch (e) {
				return null;
			}
		}

		async function decompressDeflateRaw(compressedData) {
			if (typeof DecompressionStream === 'undefined') return null;
			try {
				var ds = new DecompressionStream('deflate-raw');
				var stream = new Response(compressedData).body.pipeThrough(ds);
				return await new Response(stream).text();
			} catch (e) {
				try {
					var ds2 = new DecompressionStream('deflate');
					var stream2 = new Response(compressedData).body.pipeThrough(ds2);
					return await new Response(stream2).text();
				} catch (e2) {
					return null;
				}
			}
		}

		// Helper: Read a specific file from ZIP payload (e.g. word/document.xml, xl/worksheets/sheet1.xml)
		async function readZipEntryText(uint8Array, targetPath) {
			if (!uint8Array || uint8Array.length < 30) return null;
			try {
				var view = new DataView(uint8Array.buffer, uint8Array.byteOffset, uint8Array.byteLength);
				var offset = 0;
				while (offset < uint8Array.length - 30) {
					if (view.getUint32(offset, true) === 0x04034b50) {
						var compMethod = view.getUint16(offset + 8, true);
						var compSize = view.getUint32(offset + 18, true);
						var nameLen = view.getUint16(offset + 26, true);
						var extraLen = view.getUint16(offset + 28, true);
						var nameBytes = uint8Array.subarray(offset + 30, offset + 30 + nameLen);
						var name = new TextDecoder().decode(nameBytes);
						var dataStart = offset + 30 + nameLen + extraLen;
						var dataEnd = dataStart + compSize;

						if (name.toLowerCase() === targetPath.toLowerCase()) {
							var compressedData = uint8Array.subarray(dataStart, dataEnd);
							if (compMethod === 0) {
								return new TextDecoder().decode(compressedData);
							} else if (compMethod === 8) {
								return await decompressDeflateRaw(compressedData);
							}
						}
						offset = dataEnd > offset ? dataEnd : (offset + 1);
					} else {
						offset++;
					}
				}
			} catch (e) {
				console.warn('Zip read error:', e);
			}
			return null;
		}

		function parsePptxToHtml(slideXmls) {
			if (!slideXmls || !slideXmls.length) return '';
			var html = ['<div style="width:100%;height:100%;overflow-y:auto;padding:24px 16px;box-sizing:border-box;display:flex;flex-direction:column;align-items:center;background:#0c1317;">'];
			for (var i = 0; i < slideXmls.length; i++) {
				var xml = slideXmls[i];
				if (!xml) continue;
				var tMatches = xml.match(/<a:t\b[^>]*>([\s\S]*?)<\/a:t>/g) || [];
				var lines = [];
				for (var t = 0; t < tMatches.length; t++) {
					var rawT = tMatches[t].replace(/<a:t\b[^>]*>|<\/a:t>/g, '');
					rawT = rawT.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').trim();
					if (rawT) lines.push(rawT);
				}
				if (lines.length) {
					html.push('<div style="width:100%;max-width:760px;background:#ffffff;border-radius:8px;box-shadow:0 4px 16px rgba(0,0,0,0.4);padding:28px 32px;box-sizing:border-box;margin-bottom:18px;">');
					html.push('<div style="font-size:11px;font-weight:700;color:#00a884;text-transform:uppercase;margin-bottom:10px;letter-spacing:0.5px;">Slide ' + (i + 1) + '</div>');
					html.push('<h3 style="font-size:17px;font-weight:700;margin:0 0 10px;color:#111b21;">' + lines[0] + '</h3>');
					for (var l = 1; l < lines.length; l++) {
						html.push('<p style="font-size:13px;color:#3b4a54;margin:5px 0;line-height:1.5;">• ' + lines[l] + '</p>');
					}
					html.push('</div>');
				}
			}
			html.push('</div>');
			return html.length > 2 ? html.join('') : '';
		}

		function parseDocxToHtml(xmlStr) {
			if (!xmlStr) return '';
			var pMatches = xmlStr.match(/<w:p\b[\s\S]*?<\/w:p>/g) || [];
			var html = [];
			for (var i = 0; i < pMatches.length; i++) {
				var pStr = pMatches[i];
				var isH1 = /<w:pStyle\b[^>]*w:val="Heading1"/i.test(pStr);
				var isH2 = /<w:pStyle\b[^>]*w:val="Heading2"/i.test(pStr);
				var isH3 = /<w:pStyle\b[^>]*w:val="Heading[3-6]"/i.test(pStr);
				var rMatches = pStr.match(/<w:r\b[\s\S]*?<\/w:r>/g) || [];
				var pText = '';
				for (var j = 0; j < rMatches.length; j++) {
					var rStr = rMatches[j];
					var isBold = /<w:b\b/.test(rStr);
					var isItalic = /<w:i\b/.test(rStr);
					var tMatches = rStr.match(/<w:t\b[^>]*>([\s\S]*?)<\/w:t>/g) || [];
					for (var k = 0; k < tMatches.length; k++) {
						var tVal = tMatches[k].replace(/<w:t\b[^>]*>|<\/w:t>/g, '');
						tVal = tVal.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
						if (isBold) tVal = '<strong>' + tVal + '</strong>';
						if (isItalic) tVal = '<em>' + tVal + '</em>';
						pText += tVal;
					}
				}
				if (pText.trim()) {
					if (isH1) html.push('<h2 style="color:#111b21;margin:18px 0 8px;font-size:18px;font-weight:700;">' + pText + '</h2>');
					else if (isH2) html.push('<h3 style="color:#111b21;margin:14px 0 6px;font-size:16px;font-weight:600;">' + pText + '</h3>');
					else if (isH3) html.push('<h4 style="color:#111b21;margin:12px 0 4px;font-size:14px;font-weight:600;">' + pText + '</h4>');
					else html.push('<p style="color:#222e35;margin:8px 0;line-height:1.65;font-size:13.5px;">' + pText + '</p>');
				}
			}
			return html.join('');
		}

		// Spreadsheet preview (.xlsx, .xls, .csv) is rendered via the bundled SheetJS
		// library (see renderSpreadsheetPreview / showInAppDocModal below), which can
		// read both modern OOXML and legacy binary Excel formats directly from bytes,
		// so a hand-rolled XML/CSV parser is no longer needed here.

		// In-App Document Preview Modal Overlay (PDF, Excel, Word, Text)
		function showInAppDocModal(filename, blobUrl, savedPath, dataUri, ownedBlobUrl) {
			var existing = document.getElementById('wa-doc-modal-overlay');
			if (existing && existing.parentNode) existing.parentNode.removeChild(existing);

			// Immediately dismiss WhatsApp Web's stuck background viewer
			if (window.dismissStuckViewer) window.dismissStuckViewer();

			var ext = (filename && filename.indexOf('.') !== -1 ? filename.split('.').pop() : '').toLowerCase();
			var isPdf = ext === 'pdf';
			var isExcel = ext === 'xlsx' || ext === 'xls' || ext === 'csv';
			var isWord = ext === 'docx' || ext === 'doc' || ext === 'rtf' || ext === 'txt';
			var isPpt = ext === 'pptx' || ext === 'ppt';

			// WKWebView has no reliable built-in renderer for PDF blob URLs.
			// On macOS, render the already-saved file with PDFKit instead.
			if (isPdf && savedPath && window.showPDFPreviewNative) {
				if (window.dismissStuckViewer) window.dismissStuckViewer();
				window.showPDFPreviewNative(savedPath);
				if (ownedBlobUrl) {
					try { URL.revokeObjectURL(ownedBlobUrl); } catch (e) {}
				}
				return;
			}

			var docIcon = '📄';
			var openBtnText = '📂 Open in System App';
			var docTypeLabel = 'Document';
			if (isPdf) {
				docIcon = '📄';
				openBtnText = '📂 Open in System App';
				docTypeLabel = 'PDF Document';
			} else if (isExcel) {
				docIcon = '📊';
				openBtnText = '📊 Open in Excel / Numbers';
				docTypeLabel = 'Excel Spreadsheet';
			} else if (isWord) {
				docIcon = '📝';
				openBtnText = '📝 Open in Word / Pages';
				docTypeLabel = 'Word Document';
			} else if (isPpt) {
				docIcon = '📽️';
				openBtnText = '📽️ Open in PowerPoint / Keynote';
				docTypeLabel = 'PowerPoint Presentation';
			}

			var overlay = document.createElement('div');
			overlay.id = 'wa-doc-modal-overlay';
			overlay.style.cssText = 'position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.85);backdrop-filter:blur(10px);-webkit-backdrop-filter:blur(10px);z-index:99999999;display:flex;flex-direction:column;align-items:center;justify-content:center;padding:16px;box-sizing:border-box;animation:waFadeIn 0.2s ease;';

			var modal = document.createElement('div');
			modal.style.cssText = 'width:94%;max-width:1020px;height:92%;background:#111b21;border:1px solid rgba(255,255,255,0.14);border-radius:12px;display:flex;flex-direction:column;overflow:hidden;box-shadow:0 24px 60px rgba(0,0,0,0.85);';

			// Header
			var header = document.createElement('div');
			header.style.cssText = 'display:flex;align-items:center;justify-content:space-between;padding:10px 16px;border-bottom:1px solid rgba(255,255,255,0.08);background:#202c33;flex-shrink:0;';
			header.innerHTML = '' +
				'<div style="display:flex;align-items:center;gap:10px;min-width:0;">' +
				'  <span style="font-size:22px;">' + docIcon + '</span>' +
				'  <div style="min-width:0;">' +
				'    <strong style="font-size:13.5px;color:#e9edef;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;display:block;max-width:420px;" title="' + escapeHtml(filename) + '">' + escapeHtml(filename) + '</strong>' +
				'    <span style="font-size:11px;color:#8696a0;">' + docTypeLabel + ' · Direct Preview</span>' +
				'  </div>' +
				'</div>' +
				'<div style="display:flex;align-items:center;gap:8px;">' +
				'  <button id="wa-btn-open-preview" style="background:#00a884;color:#111b21;border:none;padding:6px 14px;border-radius:6px;font-size:12px;font-weight:600;cursor:pointer;display:flex;align-items:center;gap:4px;box-shadow:0 2px 6px rgba(0,168,132,0.3);">' +
				'    ' + openBtnText +
				'  </button>' +
				'  <button id="wa-btn-folder-doc" style="background:#2a3942;color:#e9edef;border:1px solid rgba(255,255,255,0.1);padding:6px 12px;border-radius:6px;font-size:12px;font-weight:500;cursor:pointer;">' +
				'    📂 Show in Folder' +
				'  </button>' +
				'  <button id="wa-btn-save-doc" style="background:#2a3942;color:#e9edef;border:1px solid rgba(255,255,255,0.1);padding:6px 12px;border-radius:6px;font-size:12px;font-weight:500;cursor:pointer;">' +
				'    💾 Download' +
				'  </button>' +
				'  <button id="wa-btn-close-doc" style="background:transparent;border:none;color:#8696a0;cursor:pointer;font-size:20px;padding:4px 8px;border-radius:6px;line-height:1;">✕</button>' +
				'</div>';
			modal.appendChild(header);

			// Body Container
			var body = document.createElement('div');
			body.style.cssText = 'flex:1;width:100%;height:100%;position:relative;background:#0c1317;overflow:hidden;display:flex;flex-direction:column;align-items:center;justify-content:center;';
			modal.appendChild(body);

			function triggerOpenSystem() {
				if (savedPath && window.showPDFPreviewNative && isPdf) {
					window.showPDFPreviewNative(savedPath);
				} else if (savedPath && window.openFileNative) {
					window.openFileNative(savedPath);
				} else if (window.previewDocumentNative) {
					window.previewDocumentNative(filename, dataUri || blobUrl);
				}
			}

			function renderCardFallback(hint) {
				var displayPath = savedPath || 'Downloads folder';
				body.innerHTML = '' +
					'<div style="display:flex;flex-direction:column;align-items:center;justify-content:center;padding:40px;text-align:center;">' +
					'  <div style="font-size:64px;margin-bottom:16px;">' + docIcon + '</div>' +
					'  <h2 style="color:#e9edef;font-size:18px;font-weight:600;margin:0 0 8px;max-width:540px;word-break:break-all;">' + escapeHtml(filename) + '</h2>' +
					'  <div style="color:#00a884;font-size:12px;font-weight:600;text-transform:uppercase;letter-spacing:0.5px;margin-bottom:12px;">' + docTypeLabel + ' · Saved</div>' +
					'  <p style="color:#8696a0;font-size:13px;max-width:460px;line-height:1.5;margin:0 0 16px;">' +
					(hint || ('The ' + docTypeLabel + ' is saved on your computer. Click below to open it in your default application.')) +
					'  </p>' +
					'  <div style="font-family:monospace;font-size:11px;color:#8696a0;background:rgba(255,255,255,0.06);padding:6px 14px;border-radius:6px;max-width:520px;overflow:hidden;text-overflow:ellipsis;margin-bottom:24px;border:1px solid rgba(255,255,255,0.08);">' + escapeHtml(displayPath) + '</div>' +
					'  <div style="display:flex;gap:12px;align-items:center;">' +
					'    <button id="wa-btn-card-launch" style="background:#00a884;color:#111b21;border:none;padding:10px 24px;border-radius:8px;font-size:13.5px;font-weight:600;cursor:pointer;display:flex;align-items:center;gap:6px;box-shadow:0 4px 12px rgba(0,168,132,0.3);">' +
					openBtnText +
					'    </button>' +
					'    <button id="wa-btn-card-folder" style="background:#2a3942;color:#e9edef;border:1px solid rgba(255,255,255,0.1);padding:10px 20px;border-radius:8px;font-size:13px;font-weight:500;cursor:pointer;">' +
					'📂 Show in Folder' +
					'    </button>' +
					'  </div>' +
					'</div>';
				var cardBtn = document.getElementById('wa-btn-card-launch');
				if (cardBtn) cardBtn.onclick = triggerOpenSystem;
				var folderBtn = document.getElementById('wa-btn-card-folder');
				if (folderBtn) folderBtn.onclick = function() {
					if (window.openDownloadDirNative) window.openDownloadDirNative();
				};
			}

			// Lazy-load SheetJS (xlsx.core.min.js) only when spreadsheet preview is first needed.
			var xlsxLoadPromise = null;
			function ensureXLSXLoaded() {
				// Only a library that exposes XLSX.utils is usable; an empty stub would
				// make every later XLSX.utils call throw, so treat that as "not loaded".
				if (window.XLSX && window.XLSX.utils) return Promise.resolve();
				if (xlsxLoadPromise) return xlsxLoadPromise;
				xlsxLoadPromise = new Promise(function(resolve, reject) {
					// Fetch the bundled SheetJS from the native side
					if (window.loadXLSXLibraryNative) {
						window.loadXLSXLibraryNative().then(function(jsCode) {
							try {
								  eval(jsCode);
								  // If the host page happens to expose CommonJS exports/module,
								  // the SheetJS core build initialises that object instead of a global
								  // and leaves window.XLSX as an empty stub. The direct eval above also
								  // created an eval-scoped XLSX binding, so prefer it in that case.
								  if (typeof XLSX !== 'undefined' && (!window.XLSX || !window.XLSX.utils)) {
								      window.XLSX = XLSX;
								  }
								  if (!window.XLSX || !window.XLSX.utils) {
								      throw new Error('spreadsheet library failed to initialise');
								  }
								  resolve();
							} catch (e) {
								  reject(e);
							}
						}).catch(reject);
					} else {
						reject(new Error('loadXLSXLibraryNative not available'));
					}
				});
				return xlsxLoadPromise;
			}

			// Render a parsed spreadsheet workbook (from the bundled SheetJS library) as an
			// HTML table, with a sheet-switcher tab bar when the workbook has multiple sheets.
			function renderSpreadsheetPreview(workbook, activeSheetName) {
				var sheetNames = (workbook && workbook.SheetNames) || [];
				if (!sheetNames.length) {
					renderCardFallback('This spreadsheet has no readable sheets.');
					return;
				}
				var activeName = (activeSheetName && sheetNames.indexOf(activeSheetName) !== -1) ? activeSheetName : sheetNames[0];
				var worksheet = workbook.Sheets[activeName];
				var tableHtml = sanitizeSheetHtml(XLSX.utils.sheet_to_html(worksheet, { id: 'wa-xlsx-table' }));

				var tabsHtml = '';
				if (sheetNames.length > 1) {
					tabsHtml = '<div id="wa-xlsx-tabs" style="display:flex;gap:4px;padding:8px 12px;background:#202c33;border-bottom:1px solid #2a3942;overflow-x:auto;flex-shrink:0;">';
					for (var si = 0; si < sheetNames.length; si++) {
						var name = sheetNames[si];
						var active = name === activeName;
						var safeName = name.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
						tabsHtml += '<button data-sheet="' + safeName + '" style="padding:5px 12px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;white-space:nowrap;border:1px solid ' + (active ? '#00a884' : '#2a3942') + ';background:' + (active ? '#00a884' : 'transparent') + ';color:' + (active ? '#111b21' : '#8696a0') + ';">' + safeName + '</button>';
					}
					tabsHtml += '</div>';
				}

				var tableStyle = '<style>#wa-xlsx-table{border-collapse:collapse;width:100%;font-family:system-ui,-apple-system,sans-serif;font-size:12px;color:#e9edef;}#wa-xlsx-table td,#wa-xlsx-table th{border:1px solid #2a3942;padding:6px 10px;white-space:nowrap;}#wa-xlsx-table tr:nth-child(even){background:#182229;}#wa-xlsx-table tr:nth-child(odd){background:#111b21;}</style>';

				body.innerHTML = '<div style="width:100%;height:100%;display:flex;flex-direction:column;">' + tabsHtml +
					'<div style="flex:1;overflow:auto;background:#111b21;">' + tableStyle + tableHtml + '</div></div>';

				var tabsEl = document.getElementById('wa-xlsx-tabs');
				if (tabsEl) {
					var tabBtns = tabsEl.querySelectorAll('button');
					for (var bi2 = 0; bi2 < tabBtns.length; bi2++) {
						tabBtns[bi2].onclick = function() {
							renderSpreadsheetPreview(workbook, this.getAttribute('data-sheet'));
						};
					}
				}
			}

			// Render content according to file type
			if (isPdf) {
				var pdfSrc = ownedBlobUrl || blobUrl || '';
				if ((!pdfSrc || pdfSrc.indexOf('blob:') !== 0) && dataUri && dataUri.indexOf(';base64,') !== -1) {
					pdfSrc = 'data:application/pdf;base64,' + dataUri.split(';base64,')[1];
				}
				if (pdfSrc) {
					body.innerHTML = '<iframe src="' + pdfSrc + '" style="width:100%;height:100%;border:none;background:#525659;" title="' + escapeHtml(filename) + '"></iframe>';
				} else {
					renderCardFallback();
				}
			} else if (ext === 'csv' || ext === 'xlsx' || ext === 'xls') {
				body.innerHTML = '<div style="color:#8696a0;font-size:13px;display:flex;align-items:center;gap:8px;">⏳ Loading spreadsheet preview...</div>';
				var rawXlsxB64 = (dataUri || '').indexOf(';base64,') !== -1 ? dataUri.split(';base64,')[1] : (dataUri || '');
				if (rawXlsxB64) {
					ensureXLSXLoaded().then(function() {
						try {
							// SheetJS auto-detects the real format from the bytes (OOXML zip for
							// .xlsx, binary OLE2/BIFF for legacy .xls, or plain text for .csv), so
							// one code path correctly previews all three, including .xls which the
							// previous hand-rolled parser never actually supported.
							var workbook = XLSX.read(rawXlsxB64, { type: 'base64', cellDates: true });
							renderSpreadsheetPreview(workbook);
						} catch (e) {
							renderCardFallback('Unable to render an in-app preview for this spreadsheet. Click below to open it in your default application.');
						}
					}).catch(function() {
						renderCardFallback('Unable to load spreadsheet library.');
					});
				} else {
					renderCardFallback();
				}
			} else if (ext === 'docx') {
				body.innerHTML = '<div style="color:#8696a0;font-size:13px;display:flex;align-items:center;gap:8px;">⏳ Loading Word preview...</div>';
				var uint8Doc = base64ToUint8Array(dataUri || '');
				if (uint8Doc) {
					var parsePromiseDoc = readZipEntryText(uint8Doc, 'word/document.xml');
					var timeoutPromiseDoc = new Promise(function(resolve) { setTimeout(function() { resolve(null); }, 1500); });
					Promise.race([parsePromiseDoc, timeoutPromiseDoc]).then(function(docXml) {
						if (docXml) {
							var docHtml = parseDocxToHtml(docXml);
							body.innerHTML = '' +
								'<div style="width:100%;height:100%;overflow-y:auto;padding:24px 16px;box-sizing:border-box;display:flex;justify-content:center;background:#0c1317;">' +
								'  <div style="width:100%;max-width:760px;background:#ffffff;border-radius:6px;box-shadow:0 4px 20px rgba(0,0,0,0.5);padding:40px 48px;box-sizing:border-box;min-height:90%;">' +
								docHtml +
								'  </div>' +
								'</div>';
						} else {
							renderCardFallback();
						}
					}).catch(function() {
						renderCardFallback();
					});
				} else {
					renderCardFallback();
				}
			} else if (ext === 'doc') {
				renderCardFallback('Word 97-2003 Document (.doc). Click below to open in Microsoft Word or default application.');
			} else if (ext === 'pptx') {
				body.innerHTML = '<div style="color:#8696a0;font-size:13px;display:flex;align-items:center;gap:8px;">⏳ Loading PowerPoint preview...</div>';
				var uint8Ppt = base64ToUint8Array(dataUri || '');
				if (uint8Ppt) {
					var parsePromisePpt = Promise.all([
						readZipEntryText(uint8Ppt, 'ppt/slides/slide1.xml'),
						readZipEntryText(uint8Ppt, 'ppt/slides/slide2.xml'),
						readZipEntryText(uint8Ppt, 'ppt/slides/slide3.xml'),
						readZipEntryText(uint8Ppt, 'ppt/slides/slide4.xml'),
						readZipEntryText(uint8Ppt, 'ppt/slides/slide5.xml')
					]);
					var timeoutPromisePpt = new Promise(function(resolve) { setTimeout(function() { resolve(null); }, 1500); });
					Promise.race([parsePromisePpt, timeoutPromisePpt]).then(function(slides) {
						var validSlides = slides ? slides.filter(Boolean) : [];
						if (validSlides.length) {
							body.innerHTML = parsePptxToHtml(validSlides);
						} else {
							renderCardFallback();
						}
					}).catch(function() {
						renderCardFallback();
					});
				} else {
					renderCardFallback();
				}
			} else if (ext === 'ppt') {
				renderCardFallback('PowerPoint 97-2003 Presentation (.ppt). Click below to open in PowerPoint or default application.');
			} else if (ext === 'txt' || ext === 'rtf' || ext === 'log') {
				try {
					var rawTxtB64 = (dataUri || '').indexOf(';base64,') !== -1 ? (dataUri || '').split(';base64,')[1] : (dataUri || '');
					var binTxt = atob(rawTxtB64);
					var bytesTxt = new Uint8Array(binTxt.length);
					for (var ti = 0; ti < binTxt.length; ti++) bytesTxt[ti] = binTxt.charCodeAt(ti);
					var textContent = new TextDecoder('utf-8').decode(bytesTxt);
					body.innerHTML = '<div style="width:100%;height:100%;overflow:auto;padding:24px;box-sizing:border-box;background:#111b21;color:#e9edef;font-family:monospace;font-size:13px;line-height:1.6;white-space:pre-wrap;">' +
						textContent.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;') +
						'</div>';
				} catch (e) {
					renderCardFallback();
				}
			} else {
				renderCardFallback();
			}

			overlay.appendChild(modal);
			document.body.appendChild(overlay);

			function closeDocModal() {
				window.removeEventListener('keydown', onEsc);
				if (overlay.parentNode) overlay.parentNode.removeChild(overlay);
				if (ownedBlobUrl) {
					try { URL.revokeObjectURL(ownedBlobUrl); } catch (e) {}
					ownedBlobUrl = '';
				}
				dataUri = '';
				if (window.dismissStuckViewer) window.dismissStuckViewer();
			}

			document.getElementById('wa-btn-close-doc').onclick = closeDocModal;
			overlay.onclick = function(e) {
				if (e.target === overlay) closeDocModal();
			};

			document.getElementById('wa-btn-open-preview').onclick = triggerOpenSystem;

			var btnFolder = document.getElementById('wa-btn-folder-doc');
			if (btnFolder) {
				btnFolder.onclick = function() {
					if (window.openDownloadDirNative) window.openDownloadDirNative();
				};
			}

			document.getElementById('wa-btn-save-doc').onclick = function() {
				if (dataUri && window.saveDownloadedFileNative) {
					window.saveDownloadedFileNative(filename, dataUri).then(function(p) {
						if (p) showFloatingToast('💾 Saved: ' + filename);
					});
				} else if (savedPath) {
					showFloatingToast('💾 File is saved at: ' + savedPath);
				}
			};

			var onEsc = function(e) {
				if (e.key === 'Escape' && e.isTrusted && !e._waViewerDismiss) {
					closeDocModal();
				}
			};
			window.addEventListener('keydown', onEsc);
		}
		window.showInAppDocModal = showInAppDocModal;

		// Intercept URL.createObjectURL to catch decrypted PDF/document blobs directly
		var origCreateObjectURL = URL.createObjectURL;
		URL.createObjectURL = function(blob) {
			var url = origCreateObjectURL.apply(this, arguments);
			try {
				var bType = (blob && blob.type) ? blob.type.toLowerCase() : '';
				var isDocBlob = bType.indexOf('pdf') >= 0 || bType.indexOf('officedocument') >= 0 ||
					bType.indexOf('msword') >= 0 || bType.indexOf('ms-excel') >= 0 ||
					bType.indexOf('spreadsheet') >= 0 || bType.indexOf('wordprocessing') >= 0 ||
					bType === 'text/csv' || bType === 'text/plain' ||
					(blob && (blob.type === 'application/octet-stream' || bType === '') && isRecentPDFIntent());

				if (blob && isDocBlob && !isRecentUpload() && !isRecentExplicitDownload()) {
					var name = resolveDownloadFilename(lastClickedDocName, '') || 'document';
					if (!name.includes('.')) {
						if (bType.indexOf('pdf') >= 0) name += '.pdf';
						else if (bType.indexOf('sheet') >= 0 || bType.indexOf('excel') >= 0) name += '.xlsx';
						else if (bType.indexOf('word') >= 0) name += '.docx';
						else name += '.pdf';
					}
					var isPdf = name.toLowerCase().endsWith('.pdf');
					var previewBlob = isPdf ? blob.slice(0, blob.size, 'application/pdf') : blob;
					var ownedBlobUrl = isPdf ? origCreateObjectURL(previewBlob) : '';
					var reader = new FileReader();
					reader.onloadend = function() {
						var base64data = reader.result;
						if (window.saveDownloadedFileNative) {
							window.saveDownloadedFileNative(name, base64data).then(function(savedPath) {
								showInAppDocModal(name, ownedBlobUrl, savedPath, base64data, ownedBlobUrl);
								dismissStuckViewer();
								showFloatingToast('📄 Document preview: ' + name);
							});
						} else {
							showInAppDocModal(name, ownedBlobUrl, '', base64data, ownedBlobUrl);
							dismissStuckViewer();
						}
					};
					reader.readAsDataURL(blob);
				}
			} catch (e) {}
			return url;
		};

		function handleBlobDocumentPreview(blobUrl) {
			var name = resolveDownloadFilename(lastClickedDocName, '') || 'document.pdf';
			fetch(blobUrl)
				.then(function(res) { return res.blob(); })
				.then(function(blob) {
					var isPdf = name.toLowerCase().endsWith('.pdf');
					var previewBlob = isPdf ? blob.slice(0, blob.size, 'application/pdf') : blob;
					var ownedBlobUrl = isPdf ? origCreateObjectURL(previewBlob) : '';
					var reader = new FileReader();
					reader.onloadend = function() {
						var base64data = reader.result;
						if (window.saveDownloadedFileNative) {
							window.saveDownloadedFileNative(name, base64data).then(function(savedPath) {
								showInAppDocModal(name, ownedBlobUrl, savedPath, base64data, ownedBlobUrl);
								dismissStuckViewer();
								showFloatingToast('📄 Document preview: ' + name);
							});
						} else {
							showInAppDocModal(name, ownedBlobUrl, '', base64data, ownedBlobUrl);
							dismissStuckViewer();
						}
					};
					reader.readAsDataURL(blob);
				})
				.catch(function(err) {
					console.error('Error handling blob preview:', err);
				});
		}

		// Intercept window.open for Blob URLs (PDF/Document previews) and external URLs
		var origWindowOpen = window.open;
		window.open = function(url, target, features) {
			if (url && typeof url === 'string') {
				if (url.indexOf('blob:') === 0) {
					handleBlobDocumentPreview(url);
					return null;
				}
				try {
					var parsed = new URL(url, window.location.href);
					if (!parsed.hostname.endsWith('whatsapp.com') && !parsed.hostname.endsWith('whatsapp.net') && (parsed.protocol === 'http:' || parsed.protocol === 'https:')) {
						if (window.openExternalLink) {
							window.openExternalLink(parsed.href);
							return null;
						}
					}
				} catch(err) {}
			}
			return origWindowOpen.apply(this, arguments);
		};

		// Zoom Keyboard Shortcuts (Cmd + / Cmd - / Cmd 0)
		waRunModule('zoom-shortcuts', function() {
			var currentZoom = 1.0;
			window.addEventListener('keydown', function(e) {
				if (e.metaKey || e.ctrlKey) {
					if (e.key === '=' || e.key === '+') {
						e.preventDefault();
						currentZoom = Math.min(currentZoom + 0.1, 2.0);
						document.body.style.zoom = currentZoom;
					} else if (e.key === '-') {
						e.preventDefault();
						currentZoom = Math.max(currentZoom - 0.1, 0.6);
						document.body.style.zoom = currentZoom;
					} else if (e.key === '0') {
						e.preventDefault();
						currentZoom = 1.0;
						document.body.style.zoom = currentZoom;
					}
				}
			});
		});

		// Dock Badge Unread Count Synchronizer
		waRunModule('dock-badge', function() {
			var lastBadge = null;
			function syncBadge() {
				var title = document.title || '';
				var match = title.match(/\(([^)]+)\)/);
				var badge = match ? match[1] : '';
				if (badge !== lastBadge) {
					lastBadge = badge;
					if (window.updateDockBadge) {
						window.updateDockBadge(badge);
					}
				}
			}
			var titleEl = window.waDOM.resolveFirst('pageTitle', document).node;
			if (titleEl && titleEl.nodeType) {
				try {
					new MutationObserver(syncBadge).observe(titleEl, { childList: true, characterData: true, subtree: true });
				} catch (e) {
					setInterval(syncBadge, 3000);
				}
			} else {
				setInterval(syncBadge, 3000);
			}
		});

		// Memory Optimization: Idle Garbage Collection
		waRunModule('memory-opt', function() {
			var releaseTimer = null;
			document.addEventListener('visibilitychange', function() {
				clearTimeout(releaseTimer);
				if (!document.hidden) return;
				lastClickedDocName = '';
				lastDocumentIntentAt = 0;
				// Wait a bit longer than a quick alt-tab before trimming memory, so briefly
				// switching windows doesn't repeatedly trigger native working-set trims.
				releaseTimer = setTimeout(function() {
					if (typeof window.gc === 'function') window.gc();
					if (window.releaseMemoryNative) window.releaseMemoryNative();
				}, 5000);
			});
		});

		// Debounced window resize persistence
		waRunModule('window-resize', function() {
			var resizeTimer = null;
			window.addEventListener('resize', function() {
				clearTimeout(resizeTimer);
				resizeTimer = setTimeout(function() {
					if (window.saveWindowStateNative) {
						var w = window.outerWidth || window.innerWidth;
						var h = window.outerHeight || window.innerHeight;
						if (w && h) {
							window.saveWindowStateNative(Math.round(w), Math.round(h));
						}
					}
				}, 500);
			});
		});

		// Floating HUD Toast for User Feedback. Optional action renders a
		// clickable button inside the toast (e.g. "Open folder" after a
		// download); the toast then stays interactive for a few seconds longer.
		function showFloatingToast(msg, action) {
			var toast = document.getElementById('wa-hud-toast');
			if (!toast) {
				toast = document.createElement('div');
				toast.id = 'wa-hud-toast';
				toast.style.cssText = 'position:fixed;top:16px;left:50%;transform:translateX(-50%);background:rgba(32,44,51,0.94);backdrop-filter:blur(10px);color:#00a884;border:1px solid rgba(0,168,132,0.4);border-radius:20px;padding:8px 20px;font-size:12.5px;font-weight:600;z-index:2147483647;box-shadow:0 8px 24px rgba(0,0,0,0.6);transition:all 0.22s cubic-bezier(0.16,1,0.3,1);opacity:0;display:flex;align-items:center;gap:12px;max-width:90vw;';
				var parent = document.body || document.documentElement;
				if (parent) parent.appendChild(toast);
			}
			if (!toast) return;
			var currentParent = document.body || document.documentElement;
			if (currentParent && toast.parentNode !== currentParent) {
				currentParent.appendChild(toast);
			} else if (currentParent && currentParent.lastElementChild !== toast) {
				currentParent.appendChild(toast);
			}
			toast.textContent = '';
			toast.style.pointerEvents = 'none';
			var label = document.createElement('span');
			label.textContent = msg;
			label.style.cssText = 'white-space:nowrap;overflow:hidden;text-overflow:ellipsis;';
			toast.appendChild(label);
			if (action && action.label && typeof action.onClick === 'function') {
				toast.style.pointerEvents = 'auto';
				var btn = document.createElement('button');
				btn.textContent = action.label;
				btn.style.cssText = 'background:#00a884;color:#111b21;border:none;padding:3px 10px;border-radius:12px;font-size:11px;font-weight:700;cursor:pointer;flex-shrink:0;';
				btn.onclick = function(e) {
					e.stopPropagation();
					action.onClick();
					toast.style.opacity = '0';
				};
				toast.appendChild(btn);
			}
			toast.style.opacity = '1';
			toast.style.transform = 'translateX(-50%) translateY(4px)';
			clearTimeout(toast._timer);
			toast._timer = setTimeout(function() {
				toast.style.opacity = '0';
				toast.style.transform = 'translateX(-50%) translateY(0)';
				toast.style.pointerEvents = 'none';
			}, action ? 6000 : 2500);
		}
		window.showFloatingToast = showFloatingToast;

		// Issue reporter: page errors are buffered locally (never uploaded),
		// and the Control Center offers a one-click pre-filled GitHub issue.
		// Nothing leaves the machine until the user presses Report — the
		// browser then shows the composed issue for review before submitting.
		waRunModule('diagnostics-buffer', function() {
			window.__waMeta = { ver: '__WA_APP_VERSION__', platform: '` + runtime.GOOS + `' };

			var waErrBuf = [];
			function waPushErr(kind, msg) {
				msg = String(msg || 'unknown error').slice(0, 200);
				var last = waErrBuf[waErrBuf.length - 1];
				if (last && last.m === msg) { last.n++; return; }
				waErrBuf.push({ k: kind, m: msg, n: 1 });
				if (waErrBuf.length > 25) waErrBuf.shift();
			}
			window.addEventListener('error', function(e) {
				var src = '';
				try { src = String(e.filename || '').split('/').pop(); } catch (x) {}
				waPushErr('error', (e.message || 'unknown') + ' @ ' + (src || '?') + ':' + (e.lineno || '?'));
			}, true);
			window.addEventListener('unhandledrejection', function(e) {
				var r = e.reason;
				waPushErr('unhandled', String((r && (r.stack || r.message)) || r).slice(0, 200));
			});

			function resolveMaybe(v) {
				if (v && typeof v.then === 'function') return v;
				return Promise.resolve(v);
			}
			window.openIssueReporter = function(crashTail) {
				var meta = window.__waMeta || { ver: '?', platform: '?' };
				var lines = ['WhatsApp Desk v' + meta.ver + ' (' + meta.platform + ')', ''];
				if (waErrBuf.length) {
					lines.push('Recent page errors:');
					waErrBuf.slice(-8).forEach(function(e) {
						lines.push('- [' + e.k + '] ' + e.m + (e.n > 1 ? ' (x' + e.n + ')' : ''));
					});
					lines.push('');
				} else {
					lines.push('No page errors captured.');
					lines.push('');
				}
				if (crashTail) {
					var fence = String.fromCharCode(96, 96, 96);
					lines.push('Crash log tail:');
					lines.push(fence);
					lines.push(String(crashTail).slice(0, 1200));
					lines.push(fence);
				}
				lines.push('_Submitted from the in-app reporter — please add steps to reproduce._');
				var url = 'https://github.com/vianziro/Whatsapp-Dekstop/issues/new' +
					'?title=' + encodeURIComponent('Report v' + meta.ver + ' (' + meta.platform + '): ') +
					'&body=' + encodeURIComponent(lines.join('\n').slice(0, 2500)) +
					'&labels=' + encodeURIComponent('bug');
				if (window.openExternalLink) window.openExternalLink(url);
				if (window.markCrashNotifiedNative) {
					try { resolveMaybe(window.markCrashNotifiedNative()); } catch (e) {}
				}
			};
			window.reportIssueNow = function() {
				if (window.getPendingCrashNative) {
					try {
						resolveMaybe(window.getPendingCrashNative()).then(function(t) {
							window.openIssueReporter(t || '');
						});
						return;
					} catch (e) {}
				}
				window.openIssueReporter('');
			};

			// Startup nudge, once per crash: offer reporting instead of nagging.
			setTimeout(function() {
				if (!window.getPendingCrashNative || typeof showFloatingToast !== 'function') return;
				try {
					resolveMaybe(window.getPendingCrashNative()).then(function(tail) {
						if (!tail) return;
						showFloatingToast('⚠️ Previous session crashed — tap to report', {
							label: 'Report',
							onClick: function() { window.reportIssueNow(); }
						});
					});
				} catch (e) {}
			}, 10000);
		});

		// Granular privacy profiles. WhatsApp selectors live in dom_adapter.js;
		// this module only tags resolved surfaces and applies native policy.
		waRunModule('privacy-mode', function() {
			var policy = {};
			var currentProfile = 'normal';
			var allProfiles = [];
			var manualPrivacy = false;
			var styleEl = document.createElement('style');
			styleEl.id = 'whatsapp-privacy-style';
			var surfaceMap = {
				chat_names: ['privacyChatNames', 'chatListRoot'],
				group_names: ['privacyChatNames', 'chatListRoot'],
				avatars: ['privacyAvatars', 'chatListRoot'],
				preview: ['privacyChatPreviews', 'chatListRoot'],
				timestamps: ['privacyTimestamps', 'chatListRoot'],
				unread_count: ['privacyUnreadCounts', 'chatListRoot'],
				message_text: ['privacyMessageText', 'conversationRoot'],
				images: ['privacyImages', 'conversationRoot'],
				videos: ['privacyVideos', 'conversationRoot'],
				stickers: ['privacyStickers', 'conversationRoot'],
				quoted_content: ['privacyQuotedContent', 'conversationRoot'],
				voice_note_details: ['privacyVoiceNoteDetails', 'conversationRoot'],
				header_name: ['privacyHeaderNames', 'conversationHeader'],
				header_avatar: ['privacyHeaderAvatars', 'conversationHeader'],
			header_subtitle: ['privacyHeaderSubtitles', 'conversationHeader'],
				media_viewer: ['privacyViewerMedia', 'mediaViewer']
			};
			var secondarySurfaceMap = {
				avatars: ['privacyMessageAvatars', 'conversationRoot']
			};
			var fieldLabels = {
				chat_names: 'Chat names', group_names: 'Group names', avatars: 'Avatars',
				preview: 'Message previews', timestamps: 'Timestamps', unread_count: 'Unread counts',
				message_text: 'Message text', images: 'Images', videos: 'Videos', stickers: 'Stickers',
				quoted_content: 'Quoted or replied content', voice_note_details: 'Voice-note details',
				header_name: 'Conversation name', header_avatar: 'Conversation avatar',
				header_subtitle: 'Conversation subtitle', media_viewer: 'Media viewer'
			};
			function ensureStyle() {
				if (!styleEl.parentNode) (document.head || document.documentElement).appendChild(styleEl);
				var rules = [
					'html[data-wa-privacy-manual="1"] [data-wa-privacy-surface] { filter: blur(6px) !important; }',
					'html[data-wa-privacy-reveal-all="1"] [data-wa-privacy-surface] { filter: none !important; }',
					'[data-wa-privacy-reveal="1"] [data-wa-privacy-surface], [data-wa-privacy-reveal="1"][data-wa-privacy-surface] { filter: none !important; }'
				];
				Object.keys(surfaceMap).forEach(function(key) {
					var radius = key === 'avatars' || key === 'images' || key === 'videos' || key === 'stickers' || key === 'header_avatar' ? '12px' : (key === 'media_viewer' ? '16px' : '6px');
					rules.push('html[data-wa-privacy-' + key + '="1"] [data-wa-privacy-surface~="' + key + '"] { filter: blur(' + radius + ') !important; transition: filter 0.12s ease-out !important; }');
				});
				styleEl.textContent = rules.join('\n');
			}
			function setSurface(node, key) {
				if (!node || !node.setAttribute) return;
				var current = (node.getAttribute('data-wa-privacy-surface') || '').split(/\s+/).filter(Boolean);
				if (current.indexOf(key) < 0) {
					current.push(key);
					node.setAttribute('data-wa-privacy-surface', current.join(' '));
				}
			}
			function privacyCandidates(key, root) {
				var candidates = window.waDOM.resolveCandidates(key, root);
				if (root && root.nodeType === 1 && typeof root.matches === 'function') {
					var selectors = window.waDOM.selectors(key);
					for (var i = 0; i < selectors.length; i++) {
						try {
							if (root.matches(selectors[i])) {
								if (!candidates.some(function(candidate) { return candidate.node === root; })) {
									candidates.unshift({ node: root, selector: selectors[i], priority: i });
								}
								break;
							}
						} catch (e) {}
					}
				}
				return candidates;
			}
			function scopeFor(key) {
				var spec = surfaceMap[key];
				return spec ? window.waDOM.resolveFirst(spec[1], document).node : null;
			}
			function tagPrivacySurfaces(subtree) {
				if (!window.waDOM) return;
				Object.keys(surfaceMap).forEach(function(key) {
					var spec = surfaceMap[key];
					var scope = subtree || scopeFor(key);
					if (!scope) return;
					var candidates = privacyCandidates(spec[0], scope);
					for (var i = 0; i < candidates.length; i++) {
						var node = candidates[i].node;
						if (key === 'chat_names' || key === 'group_names') {
							var row = window.waDOM.closest(node, 'chatRow', document.body).node;
							var group = row && window.waDOM.resolveCandidates('privacyGroupIndicator', row).length > 0;
							if (group === (key === 'group_names')) setSurface(node, key);
						} else setSurface(node, key);
					}
				});
				Object.keys(secondarySurfaceMap).forEach(function(key) {
					var spec = secondarySurfaceMap[key];
					var scope = subtree || window.waDOM.resolveFirst(spec[1], document).node;
					if (!scope) return;
					var candidates = privacyCandidates(spec[0], scope);
					for (var i = 0; i < candidates.length; i++) setSurface(candidates[i].node, key);
				});
			}
			function applyPolicy(nextPolicy) {
				policy = nextPolicy || {};
				var root = document.documentElement;
				if (!root) return;
				Object.keys(surfaceMap).forEach(function(key) {
					root.setAttribute('data-wa-privacy-' + key, policy[key] ? '1' : '0');
				});
				root.setAttribute('data-wa-privacy-reveal-mode', policy.reveal_mode || 'hover');
				ensureStyle();
				tagPrivacySurfaces();
			}
			function parseState(raw) {
				try { return typeof raw === 'string' ? JSON.parse(raw) : raw; } catch (e) { return null; }
			}
			function applyState(state) {
				if (!state || !state.profiles) return false;
				allProfiles = state.profiles;
				currentProfile = state.current_profile_id || 'normal';
				var active = allProfiles.filter(function(profile) { return profile.id === currentProfile; })[0];
				if (!active) return false;
				applyPolicy(active.privacy || {});
				window.dispatchEvent(new CustomEvent('wa-privacy-profile-changed', { detail: { id: currentProfile, name: active.name } }));
				return true;
			}
			window.refreshPrivacyProfiles = function() {
				if (!window.getPrivacyProfileStateNative) return Promise.resolve(false);
				return Promise.resolve(window.getPrivacyProfileStateNative()).then(function(raw) { return applyState(parseState(raw)); }).catch(function() { return false; });
			};
			window.getPrivacyProfiles = function() { return allProfiles.slice(); };
			window.getActivePrivacyProfile = function() { return currentProfile; };
			window.selectPrivacyProfile = function(id) {
				if (!window.selectPrivacyProfileNative) return Promise.resolve(false);
				return Promise.resolve(window.selectPrivacyProfileNative(String(id || ''))).then(function(raw) { return applyState(parseState(raw)); }).catch(function() { return false; });
			};
			window.copyCurrentPrivacyProfileToCustom = function() {
				if (!window.copyPrivacyProfileToCustomNative) return Promise.resolve(false);
				return Promise.resolve(window.copyPrivacyProfileToCustomNative()).then(function(raw) { return applyState(parseState(raw)); }).catch(function() { return false; });
			};
			window.resetPrivacyProfiles = function() {
				if (!window.resetPrivacyProfilesNative) return Promise.resolve(false);
				return Promise.resolve(window.resetPrivacyProfilesNative()).then(function(raw) { return applyState(parseState(raw)); }).catch(function() { return false; });
			};
			window.setCustomPrivacySurface = function(key, enabled) {
				if (currentProfile !== 'custom' || !Object.prototype.hasOwnProperty.call(surfaceMap, key) || !window.setCustomPrivacyPolicyNative) return Promise.resolve(false);
				policy[key] = !!enabled;
				applyPolicy(policy);
				return Promise.resolve(window.setCustomPrivacyPolicyNative(JSON.stringify(policy))).then(function(raw) { return applyState(parseState(raw)); }).catch(function() { return false; });
			};
			window.setCustomPrivacyReveal = function(mode, modifier) {
				if (currentProfile !== 'custom' || !window.setCustomPrivacyPolicyNative) return Promise.resolve(false);
				policy.reveal_mode = mode === 'click' || mode === 'modifier' ? mode : 'hover';
				policy.reveal_modifier = modifier === 'shift' || modifier === 'ctrl' ? modifier : 'alt';
				applyPolicy(policy);
				return Promise.resolve(window.setCustomPrivacyPolicyNative(JSON.stringify(policy))).then(function(raw) { return applyState(parseState(raw)); }).catch(function() { return false; });
			};
			window.getActivePrivacyPolicy = function() { return Object.assign({}, policy); };
			window.isPrivacyModeActive = function() { return manualPrivacy; };
			window.togglePrivacyMode = function() {
				manualPrivacy = !manualPrivacy;
				if (document.documentElement) document.documentElement.setAttribute('data-wa-privacy-manual', manualPrivacy ? '1' : '0');
				showFloatingToast(manualPrivacy ? 'Privacy Mode: enabled' : 'Privacy Mode: disabled');
				return manualPrivacy;
			};
			window.requestNativeAppLock = function() {
				if (window.requestAppLockNative) return Promise.resolve(window.requestAppLockNative());
				return Promise.resolve(false);
			};
			function revealContainer(target) {
				if (!target) return target;
				var found = window.waDOM.closest(target, 'chatRow', document.body).node ||
					window.waDOM.closest(target, 'messageContainer', document.body).node ||
					window.waDOM.closest(target, 'conversationHeader', document.body).node ||
					window.waDOM.closest(target, 'mediaViewer', document.body).node;
				return found || target;
			}
			var activeReveal = null;
			function clearReveal() {
				if (activeReveal) activeReveal.removeAttribute('data-wa-privacy-reveal');
				activeReveal = null;
			}
			function setReveal(target) {
				var next = revealContainer(target);
				if (activeReveal && activeReveal !== next) clearReveal();
				activeReveal = next;
				if (activeReveal) activeReveal.setAttribute('data-wa-privacy-reveal', '1');
			}
			document.addEventListener('mouseover', function(e) {
				if ((policy.reveal_mode || 'hover') === 'hover') setReveal(e.target);
			}, true);
			document.addEventListener('mouseout', function(e) {
				if (activeReveal && (!e.relatedTarget || !activeReveal.contains(e.relatedTarget))) clearReveal();
			}, true);
			document.addEventListener('focusin', function(e) { setReveal(e.target); }, true);
			document.addEventListener('focusout', function(e) {
				if (activeReveal && (!e.relatedTarget || !activeReveal.contains(e.relatedTarget))) clearReveal();
			}, true);
			document.addEventListener('click', function(e) {
				if ((policy.reveal_mode || 'hover') === 'click') {
					var next = revealContainer(e.target);
					if (activeReveal === next) clearReveal(); else setReveal(e.target);
				} else if (!e.target.closest || !e.target.closest('[data-wa-privacy-surface]')) clearReveal();
			}, true);
			document.addEventListener('keydown', function(e) {
				var mode = policy.reveal_mode || 'hover';
				var modifier = String(policy.reveal_modifier || 'alt').toLowerCase();
				if (mode === 'modifier' && ((modifier === 'alt' && e.altKey) || ((modifier === 'shift') && e.shiftKey) || ((modifier === 'ctrl' || modifier === 'control') && e.ctrlKey))) {
					if (document.documentElement) document.documentElement.setAttribute('data-wa-privacy-reveal-all', '1');
				}
				if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'p' || e.key === 'P')) {
					e.preventDefault(); e.stopPropagation(); window.togglePrivacyMode();
				} else if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'l' || e.key === 'L')) {
					e.preventDefault(); e.stopPropagation(); window.requestNativeAppLock();
				}
			}, true);
			document.addEventListener('keyup', function() {
				if (document.documentElement) document.documentElement.setAttribute('data-wa-privacy-reveal-all', '0');
			}, true);
			window.addEventListener('blur', function() {
				if (document.documentElement) document.documentElement.setAttribute('data-wa-privacy-reveal-all', '0');
				clearReveal();
			});
			var pendingPrivacyNodes = [];
			var tagPending = false;
			function isWithinPrivacyRoot(node) {
				return !!(
					window.waDOM.closest(node, 'chatListRoot', document.body).node ||
					window.waDOM.closest(node, 'conversationRoot', document.body).node ||
					window.waDOM.closest(node, 'mediaViewer', document.body).node
				);
			}
			function queuePrivacyNode(node) {
				if (!node || node.nodeType !== 1) return;
				if (isWithinPrivacyRoot(node)) {
					if (pendingPrivacyNodes.indexOf(node) < 0) pendingPrivacyNodes.push(node);
					return;
				}
				['chatListRoot', 'conversationRoot', 'mediaViewer'].forEach(function(key) {
					var root = window.waDOM.resolveFirst(key, node).node;
					if (root && pendingPrivacyNodes.indexOf(root) < 0) pendingPrivacyNodes.push(root);
				});
			}
			var observer = new MutationObserver(function(mutations) {
				if (document.hidden) return;
				mutations.forEach(function(mutation) {
					Array.prototype.forEach.call(mutation.addedNodes || [], queuePrivacyNode);
				});
				if (tagPending || !pendingPrivacyNodes.length) return;
				tagPending = true;
				setTimeout(function() {
					var nodes = pendingPrivacyNodes.slice();
					pendingPrivacyNodes.length = 0;
					tagPending = false;
					for (var i = 0; i < nodes.length; i++) {
						if (nodes[i].isConnected !== false) tagPrivacySurfaces(nodes[i]);
					}
				}, 150);
			});
			var privacyObserverRoot = document.getElementById('app') || document.body;
			if (privacyObserverRoot) observer.observe(privacyObserverRoot, { childList: true, subtree: true });
			window.refreshPrivacyProfiles();
		});

		// Always on Top Toggle (Cmd/Ctrl + Shift + T)
		waRunModule('always-on-top', function() {
			var isPinnedState = false;
			window.toggleAlwaysOnTop = function() {
				if (window.toggleAlwaysOnTopNative) {
					return window.toggleAlwaysOnTopNative().then(function(isPinned) {
						isPinnedState = isPinned;
						showFloatingToast(isPinned ? '📌 Always on Top: Enabled' : '📌 Always on Top: Disabled');
						return isPinned;
					});
				}
				return Promise.resolve(false);
			};
			window.isAlwaysOnTopActive = function() {
				return isPinnedState;
			};

			window.addEventListener('keydown', function(e) {
				if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 't' || e.key === 'T')) {
					e.preventDefault();
					e.stopPropagation();
					window.toggleAlwaysOnTop();
				}
			}, true);
		});

		// Reload and Refresh Functions (Cmd/Ctrl + R, Cmd/Ctrl + Shift + R, F5)
		window.reloadWhatsApp = function() {
			showFloatingToast('🔄 Reloading conversation...');
			setTimeout(function() { window.location.reload(); }, 200);
		};
		window.hardRefreshWhatsApp = function() {
			showFloatingToast('⚡ Hard refresh (clearing cache)...');
			try {
				if (window.caches && caches.keys) {
					caches.keys().then(function(names) {
						names.forEach(function(name) { caches.delete(name); });
					});
				}
			} catch (e) {}
			setTimeout(function() {
				window.location.href = window.location.origin + window.location.pathname + '?_t=' + Date.now();
			}, 200);
		};

		window.addEventListener('keydown', function(e) {
			if (e.key === 'F5' || ((e.metaKey || e.ctrlKey) && (e.key === 'r' || e.key === 'R') && !e.shiftKey && !e.altKey)) {
				e.preventDefault();
				e.stopPropagation();
				window.reloadWhatsApp();
			} else if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'r' || e.key === 'R')) {
				e.preventDefault();
				e.stopPropagation();
				window.hardRefreshWhatsApp();
			}
		}, true);

		// Audio Mute Toggle (Cmd/Ctrl + Shift + M)
		waRunModule('audio-mute', function() {
			var isMuted = false;
			window.toggleMuteAudio = function() {
				isMuted = !isMuted;
				document.querySelectorAll('audio, video').forEach(function(el) {
					el.muted = isMuted;
				});
				showFloatingToast(isMuted ? '🔇 Notification Audio: Muted' : '🔊 Notification Audio: Unmuted');
				return isMuted;
			};
			window.isAudioMuted = function() {
				return isMuted;
			};

			window.addEventListener('keydown', function(e) {
				if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'm' || e.key === 'M')) {
					e.preventDefault();
					e.stopPropagation();
					window.toggleMuteAudio();
				}
			}, true);
			document.addEventListener('play', function(e) {
				if (isMuted && e.target && (e.target.tagName === 'AUDIO' || e.target.tagName === 'VIDEO')) {
					e.target.muted = true;
				}
			}, true);
		});

		// Auto-Start at Login Toggle (Cmd/Ctrl + Shift + S)
		waRunModule('launch-on-boot', function() {
			var isAutoStartState = false;
			function refreshAutoStartState() {
				if (window.getAutoStartNative) {
					return Promise.resolve(window.getAutoStartNative()).then(function(val) {
						isAutoStartState = !!val;
						return isAutoStartState;
					}).catch(function() { return isAutoStartState; });
				}
				return Promise.resolve(isAutoStartState);
			}
			window.refreshAutoStartState = refreshAutoStartState;
			refreshAutoStartState();

			window.toggleAutoStart = function() {
				if (window.toggleAutoStartNative) {
					return window.toggleAutoStartNative().then(function(isEnabled) {
						isAutoStartState = isEnabled;
						showFloatingToast(isEnabled ? '🚀 Launch on Boot: Enabled' : '🚀 Launch on Boot: Disabled');
						return isEnabled;
					});
				}
				return Promise.resolve(false);
			};
			window.isAutoStartActive = function() {
				return isAutoStartState;
			};

			window.addEventListener('keydown', function(e) {
				if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 's' || e.key === 'S')) {
					e.preventDefault();
					e.stopPropagation();
					window.toggleAutoStart();
				}
			}, true);
		});

		// In-App Auto Updater UI and Handlers
		waRunModule('app-updater', function() {
			window.showUpdateBanner = function(latestVersion, releaseTitle, downloadUrl) {
				if (document.getElementById('wa-update-banner')) return;
				try {
					if (sessionStorage.getItem('dismissed_update_' + latestVersion) === 'true') return;
				} catch (e) {}

				if (!document.getElementById('wa-update-anim')) {
					var animStyle = document.createElement('style');
					animStyle.id = 'wa-update-anim';
					animStyle.textContent = '@keyframes waSlideDown { from { transform: translateY(-100%); opacity: 0; } to { transform: translateY(0); opacity: 1; } }' +
						'#wa-btn-update:hover { background: #029070 !important; transform: translateY(-1px); }' +
						'#wa-btn-dismiss:hover { color: #e9edef !important; }';
					document.head.appendChild(animStyle);
				}

				var banner = document.createElement('div');
				banner.id = 'wa-update-banner';
				banner.style.cssText = 'position:fixed;top:0;left:0;right:0;background:rgba(17,27,33,0.97);backdrop-filter:blur(14px);-webkit-backdrop-filter:blur(14px);border-bottom:1px solid rgba(0,168,132,0.35);padding:9px 18px;display:flex;align-items:center;justify-content:space-between;gap:12px;z-index:9999998;box-shadow:0 6px 24px rgba(0,0,0,0.6);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;color:#e9edef;font-size:13px;animation:waSlideDown 0.25s cubic-bezier(0.16,1,0.3,1);';

				var leftWrap = document.createElement('div');
				leftWrap.style.cssText = 'display:flex;align-items:center;gap:10px;min-width:0;flex:1;';

				var badge = document.createElement('span');
				badge.style.cssText = 'background:rgba(0,168,132,0.15);color:#00a884;border:1px solid rgba(0,168,132,0.35);padding:2px 8px;border-radius:12px;font-size:11px;font-weight:600;letter-spacing:0.3px;flex-shrink:0;';
				badge.textContent = 'v' + latestVersion;

				var msg = document.createElement('span');
				msg.id = 'wa-update-text';
				msg.style.cssText = 'white-space:nowrap;overflow:hidden;text-overflow:ellipsis;font-size:12.5px;color:#d1d7db;';
				var titleText = releaseTitle ? releaseTitle : ('WhatsApp Desk v' + latestVersion);
				msg.innerHTML = 'Update available: <strong style="color:#e9edef;">' + escapeHtml(titleText) + '</strong>';

				leftWrap.appendChild(badge);
				leftWrap.appendChild(msg);

				var rightWrap = document.createElement('div');
				rightWrap.style.cssText = 'display:flex;align-items:center;gap:8px;flex-shrink:0;';

				var actionsDiv = document.createElement('div');
				actionsDiv.id = 'wa-update-actions';
				actionsDiv.style.cssText = 'display:flex;align-items:center;gap:8px;';

				var btnUpdate = document.createElement('button');
				btnUpdate.id = 'wa-btn-update';
				btnUpdate.textContent = 'Update Now';
				btnUpdate.style.cssText = 'background:#00a884;color:#111b21;border:none;padding:5px 14px;border-radius:14px;font-size:12px;font-weight:600;cursor:pointer;outline:none;transition:all 0.15s ease;box-shadow:0 2px 8px rgba(0,168,132,0.3);';

				var btnDismiss = document.createElement('button');
				btnDismiss.id = 'wa-btn-dismiss';
				btnDismiss.textContent = 'Later';
				btnDismiss.style.cssText = 'background:transparent;color:#8696a0;border:none;padding:5px 10px;border-radius:14px;font-size:12px;cursor:pointer;outline:none;transition:color 0.15s ease;';

				actionsDiv.appendChild(btnUpdate);
				actionsDiv.appendChild(btnDismiss);

				var progressWrap = document.createElement('div');
				progressWrap.id = 'wa-update-progress-wrap';
				progressWrap.style.cssText = 'display:none;align-items:center;gap:10px;';

				var barTrack = document.createElement('div');
				barTrack.style.cssText = 'width:130px;height:6px;background:rgba(255,255,255,0.12);border-radius:3px;overflow:hidden;';

				var barFill = document.createElement('div');
				barFill.id = 'wa-update-progress-bar';
				barFill.style.cssText = 'width:0%;height:100%;background:#00a884;border-radius:3px;transition:width 0.18s ease;';
				barTrack.appendChild(barFill);

				var pctLabel = document.createElement('span');
				pctLabel.id = 'wa-update-progress-pct';
				pctLabel.style.cssText = 'font-size:11.5px;color:#00a884;font-weight:600;min-width:32px;text-align:right;';
				pctLabel.textContent = '0%';

				progressWrap.appendChild(barTrack);
				progressWrap.appendChild(pctLabel);

				rightWrap.appendChild(actionsDiv);
				rightWrap.appendChild(progressWrap);

				banner.appendChild(leftWrap);
				banner.appendChild(rightWrap);
				var bannerParent = document.body || document.documentElement;
				if (bannerParent) bannerParent.appendChild(banner);

				try {
					if ((!window.isNotificationsEnabled || window.isNotificationsEnabled()) && window.sendNativeNotification) {
						var notifTitle = 'Update Available';
						var notifBody = 'WhatsApp Desk v' + latestVersion + ' is available. Click to update the application.';
						window.sendNativeNotification(notifTitle, notifBody);
					}
				} catch (e) {}

				btnUpdate.onclick = function() {
					if (!downloadUrl && window.triggerCheckForUpdate) {
						window.triggerCheckForUpdate();
						return;
					}
					actionsDiv.style.display = 'none';
					progressWrap.style.display = 'flex';
					btnUpdate.textContent = 'Updating...';
					msg.style.color = '#d1d7db';
					msg.textContent = 'Downloading update package...';
					if (window.startUpdateNative) {
						window.startUpdateNative(downloadUrl);
					}
				};

				btnDismiss.onclick = function() {
					sessionStorage.setItem('dismissed_update_' + latestVersion, 'true');
					if (banner.parentNode) {
						banner.parentNode.removeChild(banner);
					}
				};
			};

			window.onUpdateProgress = function(pct) {
				var bar = document.getElementById('wa-update-progress-bar');
				var label = document.getElementById('wa-update-progress-pct');
				var msg = document.getElementById('wa-update-text');
				if (bar) bar.style.width = pct + '%';
				if (label) label.textContent = pct + '%';
				if (msg) msg.textContent = 'Downloading update package... ' + pct + '%';
			};

			window.onUpdateStatus = function(statusMsg) {
				var msg = document.getElementById('wa-update-text');
				if (msg) msg.textContent = statusMsg;
			};

			window.onUpdateError = function(errMsg) {
				var actions = document.getElementById('wa-update-actions');
				var prog = document.getElementById('wa-update-progress-wrap');
				var msg = document.getElementById('wa-update-text');
				var retry = document.getElementById('wa-btn-update');
				if (actions) actions.style.display = 'flex';
				if (prog) prog.style.display = 'none';
				if (retry) retry.textContent = 'Retry';
				if (msg) {
					msg.textContent = 'Update failed: ' + (errMsg || 'Unknown error');
					msg.style.color = '#ff8a80';
				}
				showFloatingToast('❌ Failed to update: ' + errMsg);
			};

			// Manual Check Function and Shortcut (Cmd/Ctrl + Shift + U)
			window.triggerCheckForUpdate = function() {
				showFloatingToast('🔍 Checking for updates...');
				if (window.checkForUpdateNative) {
					return window.checkForUpdateNative(true).then(function(res) {
						if (res && res.available) {
							window.showUpdateBanner(res.latest_version, res.release_title, res.download_url);
						} else if (res && res.check_error) {
							showFloatingToast('⚠️ Update check failed: ' + res.check_error);
						} else {
							var cur = (res && res.current_version) ? res.current_version : '__WA_APP_VERSION__';
							showFloatingToast('✅ WhatsApp Desk is up to date (v' + cur + ')');
						}
						return res;
					}).catch(function() {
						showFloatingToast('⚠️ Unable to check for updates at this time.');
					});
				}
				return Promise.resolve(null);
			};

			window.addEventListener('keydown', function(e) {
				if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'u' || e.key === 'U')) {
					e.preventDefault();
					e.stopPropagation();
					window.triggerCheckForUpdate();
				}
			}, true);
		});

		// Dynamic Responsive Desktop Layout (enables seamless shrinking and expanding)
		waRunModule('responsive-css', function() {
			var respStyle = document.createElement('style');
			respStyle.id = 'whatsapp-desktop-responsive';
			respStyle.textContent = '' +
				'html, body, #app { width: 100% !important; height: 100% !important; min-width: 0 !important; overflow: hidden !important; -webkit-font-smoothing: antialiased; }' +
				'#app > div, #app .two { width: 100% !important; height: 100% !important; min-width: 0 !important; max-width: 100% !important; top: 0 !important; margin: 0 !important; border-radius: 0 !important; }' +
				'[data-testid="status-v3"] { min-width: 0 !important; width: 100% !important; height: 100% !important; }' +
				'@media screen and (min-width: 641px) {' +
				'  #pane-side, div[data-testid="chat-list"] { min-width: 200px !important; -webkit-overflow-scrolling: touch !important; }' +
				'  #main { min-width: 240px !important; -webkit-overflow-scrolling: touch !important; }' +
				'}' +
				'@media screen and (max-width: 640px) {' +
				'  #pane-side, div[data-testid="chat-list"], #main { min-width: 0 !important; }' +
				'}';

			// Once <head> exists the style never needs re-injection, so poll only
			// via a cheap head observer instead of an endless 2.5s interval.
			function injectResponsive() {
				var targetHead = document.head || (document.documentElement && document.documentElement.querySelector && document.documentElement.querySelector('head'));
				if (targetHead && !document.getElementById('whatsapp-desktop-responsive')) {
					targetHead.appendChild(respStyle);
					try { observer.disconnect(); } catch (e) {}
				}
			}
			var observer = new MutationObserver(injectResponsive);
			if (document.head) {
				injectResponsive();
			} else if (document.documentElement && document.documentElement.nodeType) {
				try { observer.observe(document.documentElement, { childList: true }); } catch (e) {}
			} else if (document && document.nodeType) {
				try { observer.observe(document, { childList: true, subtree: true }); } catch (e) {}
			}
			document.addEventListener('DOMContentLoaded', function() {
				injectResponsive();
				try { observer.disconnect(); } catch (e) {}
			}, { once: true });
		});

		// Native Spell Check for textareas (macOS NSSpellChecker, Windows ISpellCheckProvider, Linux GTK)
		waRunModule('spellcheck', function() {
			var spellCheckEnabled = true;
			var spellCheckLang = 'auto';

			function applySpellCheck(el) {
				if (!el || el.nodeType !== 1 || el.dataset.spellCheckInitialized) return;
				el.dataset.spellCheckInitialized = 'true';
				el.spellcheck = spellCheckEnabled;
				if (spellCheckLang !== 'auto') {
					el.lang = spellCheckLang;
				}
			}

			function enableSpellCheckOnTextareas(scope) {
				var selector = 'textarea[contenteditable="true"], div[contenteditable="true"][role="textbox"], textarea';
				var root = (scope && scope.querySelectorAll) ? scope : document;
				try {
					if (scope && scope.matches && scope.matches(selector)) applySpellCheck(scope);
					var textareas = root.querySelectorAll(selector);
					for (var i = 0; i < textareas.length; i++) applySpellCheck(textareas[i]);
				} catch (e) {}
			}

			function initSpellCheck() {
				// Initial enable
				enableSpellCheckOnTextareas();

				// Watch for new textareas (WhatsApp Web is SPA). The chat list is
				// virtualized, so scrolling continuously adds and removes rows. The old
				// version ran a document-wide querySelectorAll for every single added
				// node, which is what made long chat-list scrolls stutter. Coalesce to
				// one pass per animation frame and only for subtrees that can hold
				// editable text.
				var spellCheckScheduled = false;
				var pendingSpellRoots = [];
				function spellCheckRelevant(node) {
					if (!node || node.nodeType !== 1) return false;
					try {
						return !!(node.matches && node.matches('[contenteditable="true"], textarea')) ||
							!!(node.querySelector && node.querySelector('[contenteditable="true"], textarea'));
					} catch (e) {
						return false;
					}
				}
				function runSpellCheckScan() {
					spellCheckScheduled = false;
					var roots = pendingSpellRoots.splice(0, pendingSpellRoots.length);
					if (shouldPauseBackgroundWork()) return;
					for (var r = 0; r < roots.length; r++) enableSpellCheckOnTextareas(roots[r]);
				}
				function scheduleSpellCheck() {
					if (spellCheckScheduled || pendingSpellRoots.length === 0) return;
					spellCheckScheduled = true;
					requestAnimationFrame(runSpellCheckScan);
				}
				var observer = new MutationObserver(function(mutations) {
					if (shouldPauseBackgroundWork()) return;
					for (var i = 0; i < mutations.length && pendingSpellRoots.length < 12; i++) {
						var added = mutations[i].addedNodes;
						for (var j = 0; j < added.length && pendingSpellRoots.length < 12; j++) {
							if (spellCheckRelevant(added[j])) pendingSpellRoots.push(added[j]);
						}
					}
					scheduleSpellCheck();
				});
				var target = document.body || document.documentElement || document;
				if (target && target.nodeType) {
					try { observer.observe(target, { childList: true, subtree: true }); } catch (e) {}
				}

				// Also re-check on navigation
				var lastUrl = location.href;
				setInterval(function() {
					if (location.href !== lastUrl) {
						lastUrl = location.href;
						setTimeout(enableSpellCheckOnTextareas, 300);
					}
				}, 1000);
			}

			// Expose toggle for settings
			window.toggleSpellCheck = function(enabled) {
				spellCheckEnabled = !!enabled;
				enableSpellCheckOnTextareas();
				if (window.setSpellCheckEnabledNative) {
					window.setSpellCheckEnabledNative(spellCheckEnabled);
				}
			};

			window.setSpellCheckLanguage = function(lang) {
				spellCheckLang = lang;
				enableSpellCheckOnTextareas();
			};

			if (document.readyState === 'loading') {
				document.addEventListener('DOMContentLoaded', initSpellCheck);
			} else {
				initSpellCheck();
			}
		});

		// Context Menu: Search/Translate selected text
		waRunModule('context-menu', function() {
			var contextMenu = null;
			var lastSelection = '';
			var lastSelectionRect = null;

			function createContextMenu() {
				if (contextMenu) return;
				contextMenu = document.createElement('div');
				contextMenu.id = 'wa-context-menu';
				contextMenu.style.cssText = 'position:fixed;z-index:9999999;background:#202c33;border:1px solid #2a3942;border-radius:8px;padding:6px 0;box-shadow:0 8px 24px rgba(0,0,0,0.4);min-width:180px;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;font-size:13px;color:#e9edef;';
				contextMenu.innerHTML = '' +
					'<div class="wa-cm-item" data-action="search" style="padding:8px 16px;cursor:pointer;display:flex;align-items:center;gap:10px;">' +
					'  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color:#00a884;"><circle cx="11" cy="11" r="8"></circle><line x1="21" y1="21" x2="16.65" y2="16.65"></line></svg>' +
					'  <span>Search on Google</span>' +
					'</div>' +
					'<div class="wa-cm-item" data-action="translate" style="padding:8px 16px;cursor:pointer;display:flex;align-items:center;gap:10px;">' +
					'  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color:#00a884;"><path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8.9h.5a8.48 8.48 0 0 1 8 8v.5z"></path><line x1="12" y1="12" x2="12" y2="12"></line></svg>' +
					'  <span>Translate</span>' +
					'</div>' +
					'<hr style="margin:6px 8px;border:none;border-top:1px solid #2a3942;">' +
					'<div class="wa-cm-item" data-action="copy" style="padding:8px 16px;cursor:pointer;display:flex;align-items:center;gap:10px;">' +
					'  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="color:#8696a0;"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>' +
					'  <span>Copy</span>' +
					'</div>';
				var parent = document.body || document.documentElement;
				if (parent) parent.appendChild(contextMenu);

				contextMenu.querySelectorAll('.wa-cm-item').forEach(function(item) {
					item.addEventListener('mouseenter', function() {
						this.style.background = '#2a3942';
					});
					item.addEventListener('mouseleave', function() {
						this.style.background = 'transparent';
					});
					item.addEventListener('click', function() {
						var action = this.dataset.action;
						handleContextAction(action);
						hideContextMenu();
					});
				});

				document.addEventListener('click', hideContextMenu, true);
				document.addEventListener('scroll', hideContextMenu, true);
			}

			function showContextMenu(x, y, text) {
				createContextMenu();
				lastSelection = text;
				contextMenu.style.left = x + 'px';
				contextMenu.style.top = y + 'px';
				contextMenu.style.display = 'block';
			}

			function hideContextMenu() {
				if (contextMenu) {
					contextMenu.style.display = 'none';
				}
			}

			function handleContextAction(action) {
				if (!lastSelection) return;
				var encoded = encodeURIComponent(lastSelection);
				if (action === 'search') {
					window.openExternalLink && window.openExternalLink('https://www.google.com/search?q=' + encoded);
				} else if (action === 'translate') {
					window.openExternalLink && window.openExternalLink('https://translate.google.com/?sl=auto&tl=id&text=' + encoded + '&op=translate');
				} else if (action === 'copy') {
					navigator.clipboard.writeText(lastSelection).then(function() {
						if (window.showFloatingToast) window.showFloatingToast('📋 Copied to clipboard');
					});
				}
			}

			function getSelectedText() {
				var selection = window.getSelection();
				if (!selection || selection.rangeCount === 0) return '';
				var text = selection.toString().trim();
				return text.length > 0 && text.length < 500 ? text : '';
			}

			function onContextMenu(e) {
				var text = getSelectedText();
				if (text) {
					e.preventDefault();
					showContextMenu(e.clientX, e.clientY, text);
				}
			}

			document.addEventListener('contextmenu', onContextMenu, true);

			// Also show on long-press for touch devices
			var longPressTimer = null;
			document.addEventListener('touchstart', function(e) {
				var text = getSelectedText();
				if (text) {
					longPressTimer = setTimeout(function() {
						var touch = e.touches[0];
						showContextMenu(touch.clientX, touch.clientY, text);
					}, 500);
				}
			}, { passive: true });
			document.addEventListener('touchend', function() {
				if (longPressTimer) clearTimeout(longPressTimer);
			});
			document.addEventListener('touchmove', function() {
				if (longPressTimer) clearTimeout(longPressTimer);
			});
		});

		// Automatic Download & Document Preview Interceptor for Chat Files & Media
		waRunModule('document-viewer', function() {
			var activeDownloadKeys = Object.create(null);
			// Paths the saver has already reported in this session. The Go saver
			// returns the path of an existing byte-identical file instead of
			// writing a second copy, so seeing the same path a second time means
			// the content was already on disk and the toast should say so.
			var savedDownloadPaths = Object.create(null);

			function downloadRequestKey(href, filename) {
				return String(filename || '') + '\n' + String(href || '');
			}

			function releaseDownloadRequest(requestKey, immediately) {
				delete activeDownloadKeys[requestKey];
			}

			// Toast action: opens the downloads folder in Finder/Explorer.
			function openFolderAction() {
				if (!window.openDownloadDirNative) return null;
				return {
					label: 'Open folder',
					onClick: function() { window.openDownloadDirNative(); }
				};
			}

			function markDownloadComplete(requestKey, savedPath) {
				var completedRequest = { status: 'complete', savedPath: savedPath };
				activeDownloadKeys[requestKey] = completedRequest;
				// Tell the badge layer this filename is now on disk so the next
				// scan badges it without a redundant native stat.
				var savedBase = (savedPath || '').split(/[\\/]/).pop();
				if (savedBase && window.__waMarkSaved) window.__waMarkSaved(savedBase);
				// Retain only the tiny path entry, never the Blob or base64 payload.
				setTimeout(function() {
					if (activeDownloadKeys[requestKey] === completedRequest) {
						delete activeDownloadKeys[requestKey];
					}
				}, 300000);
			}

			function isDocumentFileName(name) {
				if (!name) return false;
				var ext = name.toLowerCase();
				return ext.endsWith('.pdf') || ext.endsWith('.doc') || ext.endsWith('.docx') ||
					   ext.endsWith('.xls') || ext.endsWith('.xlsx') || ext.endsWith('.ppt') ||
					   ext.endsWith('.pptx') || ext.endsWith('.txt') || ext.endsWith('.csv') ||
					   ext.endsWith('.rtf');
			}

			function captureDownload(href, filename, shouldAutoOpen) {
				filename = resolveDownloadFilename(filename, '');
				var isDoc = isDocumentFileName(filename);
				if (shouldAutoOpen === undefined) {
					shouldAutoOpen = isDoc;
				}
				var requestKey = downloadRequestKey(href, filename);
				var existingRequest = activeDownloadKeys[requestKey];
				if (existingRequest && existingRequest.status === 'downloading') {
					return;
				}
				activeDownloadKeys[requestKey] = { status: 'downloading' };

				showFloatingToast(isDoc ? ('📄 Opening preview: ' + filename + '...') : ('⏳ Downloading: ' + filename + '...'));
				fetch(href)
					.then(function(response) {
						if (!response) return null;
						filename = resolveDownloadFilename(filename, response.headers.get('Content-Disposition'));
						return response.blob();
					})
					.then(function(blob) {
						if (!blob) return;
						var isPdf = filename.toLowerCase().endsWith('.pdf');
						var previewBlob = isPdf ? blob.slice(0, blob.size, 'application/pdf') : blob;
						var ownedBlobUrl = isPdf ? origCreateObjectURL(previewBlob) : '';
						var reader = new FileReader();
						reader.onloadend = function() {
							var base64data = reader.result;
							if (window.saveDownloadedFileNative) {
								window.saveDownloadedFileNative(filename, base64data).then(function(savedPath) {
									if (savedPath) {
										var alreadySaved = savedDownloadPaths[savedPath] === true;
										savedDownloadPaths[savedPath] = true;
										markDownloadComplete(requestKey, savedPath);
										if (shouldAutoOpen) {
											showInAppDocModal(filename, ownedBlobUrl || href, savedPath, base64data, ownedBlobUrl);
											if (window.dismissStuckViewer) window.dismissStuckViewer();
											showFloatingToast(alreadySaved ? ('📄 Already saved: ' + filename) : ('📄 Preview opened: ' + filename), openFolderAction());
										} else {
											showFloatingToast(alreadySaved ? ('💾 File already saved: ' + filename) : ('💾 Saved successfully: ' + filename), openFolderAction());
										}
									} else {
										if (ownedBlobUrl) URL.revokeObjectURL(ownedBlobUrl);
										showFloatingToast('❌ Failed to save file.');
										releaseDownloadRequest(requestKey);
									}
								}).catch(function() {
									if (ownedBlobUrl) URL.revokeObjectURL(ownedBlobUrl);
									showFloatingToast('❌ Error saving file.');
									releaseDownloadRequest(requestKey);
								});
							} else {
								if (ownedBlobUrl) URL.revokeObjectURL(ownedBlobUrl);
								releaseDownloadRequest(requestKey);
							}
						};
						reader.onerror = function() {
							if (ownedBlobUrl) URL.revokeObjectURL(ownedBlobUrl);
							releaseDownloadRequest(requestKey);
						};
						reader.readAsDataURL(blob);
					})
					.catch(function(err) {
						console.error('Download intercept fetch error:', err);
						releaseDownloadRequest(requestKey);
					});
			}

			var forwardingDocumentDownload = false;
			var pendingViewerDownloadClick = false;
			var viewerDownloadSelector = window.waDOM.selectors('mediaViewerDownloadControl').join(',');
			function isExplicitDownloadMenuItem(target) {
				if (!target || !target.closest) return false;
				var item = target.closest('[role="menuitem"]');
				if (!item) return false;
				var label = (item.getAttribute('aria-label') || item.getAttribute('title') || item.innerText || '')
					.replace(/\s+/g, ' ').trim();
				return /^(download|unduh)$/i.test(label);
			}
			document.addEventListener('click', function(e) {
				var target = e.target;
				if (target && target.closest &&
					(target.closest(viewerDownloadSelector) || isExplicitDownloadMenuItem(target))) {
					lastExplicitDownloadAt = Date.now();
				}
			}, true);

			function findVisibleViewerDownloadControl() {
				var candidates = window.waDOM.resolveCandidatesInDOMOrder('mediaViewerDownloadControl', document);
				var best = null;
				var bestScore = -1;
				for (var i = 0; i < candidates.length; i++) {
					var raw = candidates[i].node;
					if (raw.closest && raw.closest('#wa-doc-modal-overlay')) continue;
					var control = (raw.closest && raw.closest('button, a, [role="button"]')) || raw;
					var rect = control.getBoundingClientRect();
					if (rect.width < 8 || rect.height < 8 || rect.bottom <= 0 || rect.right <= 0 ||
						rect.top >= window.innerHeight || rect.left >= window.innerWidth) continue;
					var style = window.getComputedStyle(control);
					if (style.display === 'none' || style.visibility === 'hidden' || Number(style.opacity) === 0) continue;

					var score = 0;
					if (rect.top < window.innerHeight * 0.3) score += 4;
					if (rect.left > window.innerWidth * 0.55) score += 3;
					if (control.closest && control.closest('[role="dialog"], [data-testid*="viewer"], header, [role="toolbar"]')) score += 5;
					if (score > bestScore) {
						best = control;
						bestScore = score;
					}
				}
				return bestScore >= 4 ? best : null;
			}

			function triggerVisibleViewerDownload() {
				if (pendingViewerDownloadClick || !isRecentPDFIntent()) return false;
				var control = findVisibleViewerDownloadControl();
				if (!control) return false;
				pendingViewerDownloadClick = true;
				control.click();
				setTimeout(function() { pendingViewerDownloadClick = false; }, 1500);
				return true;
			}

			function findDocumentDownloadControl(start) {
				var node = start;
				for (var depth = 0; node && node !== document.body && depth < 12; depth++, node = node.parentElement) {
					var candidates = window.waDOM.resolveCandidatesInDOMOrder('documentDownloadControl', node);
					var found = candidates.length ? candidates[0].node : null;
					if (found) return found.closest('button, a, [role="button"]') || found;
				}
				return null;
			}

			// Hook 1: Override HTMLAnchorElement.prototype.click (programmatic downloads)
			var originalAnchorClick = HTMLAnchorElement.prototype.click;
			HTMLAnchorElement.prototype.click = function() {
				var downloadAttr = this.getAttribute('download');
				var href = this.href || this.getAttribute('href');
				if ((downloadAttr !== null || this.download) && href && (href.indexOf('blob:') === 0 || href.indexOf('data:') === 0)) {
					var name = resolveDownloadFilename(downloadAttr || this.download || lastClickedDocName || 'whatsapp_media', '');
					// An explicit download anchor means save only. Opening a document
					// preview is reserved for clicking the document itself.
					lastExplicitDownloadAt = Date.now();
					captureDownload(href, name, false);
					return;
				}
				return originalAnchorClick.apply(this, arguments);
			};

			// Hook 2: User click event capturing (direct clicks on <a> with download)
			document.addEventListener('click', function(e) {
				var target = e.target;
				while (target && target !== document.body) {
					if (target.tagName === 'A') {
						var downloadAttr = target.getAttribute('download');
						var href = target.href || target.getAttribute('href');
						if ((downloadAttr !== null || target.download) && href && (href.indexOf('blob:') === 0 || href.indexOf('data:') === 0)) {
							e.preventDefault();
							e.stopPropagation();
							var name = resolveDownloadFilename(downloadAttr || target.download || lastClickedDocName || 'whatsapp_media', '');
							// The user clicked Download directly: do not open a second preview.
							lastExplicitDownloadAt = Date.now();
							captureDownload(href, name, false);
							return;
						}
					}
					target = target.parentElement;
				}
			}, true);

			// Hook 3: Watch document bubble clicks in chat to handle viewer spinner
			document.addEventListener('click', function(e) {
				if (forwardingDocumentDownload) return;
				var el = e.target;
				if (!el) return;

				// Completely ignore clicks inside media-viewer or custom modal overlay
				if (typeof el.closest === 'function') {
					if (window.waDOM.closest(el, 'mediaViewer').status === 'found' || el.closest('#wa-doc-modal-overlay')) {
						return;
					}
				}

				var foundName = extractDocumentName(el);
				var clickedDoc = !!foundName;

				if (clickedDoc) {
					lastClickedDocName = foundName;
					lastDocumentIntentAt = Date.now();
					if (isDocumentFileName(foundName)) {
						var directDownload = findDocumentDownloadControl(el);
						if (directDownload && !directDownload.contains(el)) {
							e.preventDefault();
							e.stopImmediatePropagation();
							forwardingDocumentDownload = true;
							directDownload.click();
							forwardingDocumentDownload = false;
							return;
						}
					}

					var checkCount = 0;
					var checkTimer = setInterval(function() {
						if (shouldPauseBackgroundWork()) {
							clearInterval(checkTimer);
							return;
						}
						checkCount++;
						if (checkCount > 30) {
							clearInterval(checkTimer);
							return;
						}

						if (triggerVisibleViewerDownload()) {
							clearInterval(checkTimer);
						}
					}, 200);
				}
			}, true);

			// Hook 4: MutationObserver to auto-dismiss stuck media viewer and trigger download/preview.
			// This observes the whole document body (subtree), which also churns heavily while
			// the chat list is scrolled, so coalesce to at most one check per animation frame
			// instead of running on every individual mutation batch.
			var viewerCheckScheduled = false;
			var viewerObserver = new MutationObserver(function() {
				if (shouldPauseBackgroundWork() || !isRecentPDFIntent() || viewerCheckScheduled) return;
				viewerCheckScheduled = true;
				requestAnimationFrame(function() {
					viewerCheckScheduled = false;
					if (!isRecentPDFIntent()) return;
					if (!document.getElementById('wa-doc-modal-overlay')) triggerVisibleViewerDownload();
				});
			});

			function initViewerObserver() {
				var target = document.body || document.documentElement || document;
				if (target && target.nodeType) {
					try { viewerObserver.observe(target, { childList: true, subtree: true }); } catch (e) {}
				} else {
					document.addEventListener('DOMContentLoaded', initViewerObserver, { once: true });
				}
			}
			initViewerObserver();
		});

		// "Saved to disk" badges on the Media/Docs panel. WhatsApp has no notion
		// of local downloads, so bridge it: for each document/media item shown in
		// the all-chats panel, check whether the same filename exists in the
		// configured downloads folder and tag it with a small green check.
		waRunModule('saved-badges', function() {
			var savedScanQueued = false;
			var lastSavedScanAt = 0;
			var savedCache = {};
			var savedPending = {};
			var badgeStyle = 'display:inline-flex;align-items:center;gap:2px;margin-left:6px;padding:0 6px;border-radius:8px;' +
				'font-size:10px;font-weight:600;line-height:14px;vertical-align:middle;background:rgba(6,174,116,.16);color:#06ae74;';

			function fileExistsOnDisk(name) {
				if (!name || !window.checkFileExistsNative) return Promise.resolve(false);
				if (name in savedCache) return Promise.resolve(savedCache[name]);
				// Coalesce concurrent lookups for the same name: repeated scans
				// while a check is in flight must not spam the native binding.
				if (savedPending[name]) return savedPending[name];
				var p = window.checkFileExistsNative(name).then(function(exists) {
					delete savedPending[name];
					savedCache[name] = !!exists;
					return !!exists;
				}).catch(function() {
					delete savedPending[name];
					return false;
				});
				savedPending[name] = p;
				return p;
			}

			// Called by the download path so a just-saved file badges instantly.
			window.__waMarkSaved = function(name) {
				if (name) savedCache[name] = true;
			};

			function findFileNameElement(el) {
				if (!el) return null;
				if (el.getAttribute && el.getAttribute('title')) return el;
				return el.querySelector && el.querySelector('span[title], div[title]');
			}

			function decorateItem(el, name, filenameEl) {
				if (el.__waSavedBadge) return;
				fileExistsOnDisk(name).then(function(exists) {
					if (!exists) return;
					el.__waSavedBadge = true;
					var badge = document.createElement('span');
					badge.className = 'wa-saved-badge';
					badge.setAttribute('aria-label', 'Already saved to downloads folder');
					badge.style.cssText = badgeStyle;
					badge.textContent = '✓ Saved';
					// Attach the badge to the filename element so it follows the
					// incoming/outgoing bubble instead of landing at the row's left edge.
					var host = filenameEl || el.querySelector('[data-testid="cell-frame-container"], .copyable-text') || el;
					host.style.position = host.style.position || 'relative';
					host.appendChild(badge);
				});
			}

			function itemFileName(el) {
				var titleEl = findFileNameElement(el);
				var t = titleEl ? (titleEl.getAttribute('title') || '') : '';
				if (!t) return '';
				var m = t.match(/([^\n\r<>]{1,180}\.(pdf|docx?|xlsx?|pptx?|txt|csv|rtf|zip|mp4|mkv|mov|mp3|wav|jpe?g|png|webp|heic))\b/i);
				return m ? m[1].trim() : '';
			}

			function scanPanel() {
				// Only scan where items can actually be seen: the open media/docs
				// panel (dialog/viewer) or the current chat pane. Scanning the
				// whole document on every chat-list mutation is exactly the
				// background churn this app is supposed to avoid.
				var scopeCandidates = window.waDOM.resolveCandidatesInDOMOrder('savedFileScanPanel', document);
				var scope = (scopeCandidates.length && scopeCandidates[0].node) ||
					window.waDOM.resolveFirst('conversationRoot', document).node;
				if (!scope) return;
				var rows = window.waDOM.resolveCandidatesInDOMOrder('savedFileMessageRow', scope);
				for (var i = 0; i < rows.length; i++) {
					var row = rows[i].node;
					if (row.__waSavedBadge) continue;
					var name = itemFileName(row);
					if (name) decorateItem(row, name, findFileNameElement(row));
				}
			}

			function scheduleScan() {
				if (savedScanQueued || shouldPauseBackgroundWork()) return;
				// Hard throttle: the panel observer fires on every DOM mutation
				// while WhatsApp virtualizes lists; 2s between scans is plenty
				// for a "saved" badge that is purely informational.
				var now = Date.now();
				if (now - lastSavedScanAt < 2000) return;
				savedScanQueued = true;
				requestAnimationFrame(function() {
					savedScanQueued = false;
					lastSavedScanAt = Date.now();
					scanPanel();
				});
			}

			// Observe only where badges can appear (open dialog/viewer or the
			// chat pane). Chat-list churn in #pane-side never needs a rescan,
			// so ignore mutations outside the relevant scope entirely.
			function panelMutationRelevant(muts) {
				for (var i = 0; i < muts.length; i++) {
					var t = muts[i].target;
					if (t && t.closest) {
						try {
							if (t.closest('#main, [role="dialog"], [data-testid="media-viewer"], #wa-doc-modal-overlay')) return true;
						} catch (e) {}
					}
				}
				return false;
			}
			var panelObserver = new MutationObserver(function(muts) {
				if (panelMutationRelevant(muts)) scheduleScan();
			});
			function watchRoot() {
				var root = document.body || document.documentElement || document;
				if (root && root.nodeType) {
					try { panelObserver.observe(root, { childList: true, subtree: true }); } catch (e) {}
				}
			}
			watchRoot();
			document.addEventListener('DOMContentLoaded', watchRoot, { once: true });
			document.addEventListener('click', function(e) {
				// Rescan when the user opens the media/docs panel from the toolbar.
				if (e.target && e.target.closest && e.target.closest('[data-testid="chat-menu"], [data-icon="default-image"], [data-icon="docs"], [data-icon="image"]')) {
					setTimeout(scheduleScan, 300);
				}
			}, true);
		});

		// Theme Manager, In-Flow Header Toolbar Button & Control Center Modal
		waRunModule('settings-modal', function() {
			var isMac = (__WA_GOOS === 'darwin') || (navigator.platform && navigator.platform.toUpperCase().indexOf('MAC') >= 0);
			var currentTheme = 'dark';
			var themeChoiceVersion = 0;
			var themeLoadStarted = false;
			var themeReloadTimer = null;
			var themeReapplyTimers = [];
			var themeStyle = null;
			var mediaPermissionCard = null;
			// Keep the engine's native MediaQueryList intact. Replacing matchMedia with
			// a partial object breaks framework listeners on some WebView2/WebKitGTK
			// versions and was the main cross-platform difference in theme switching.
			var origMatchMedia = window.matchMedia ? window.matchMedia.bind(window) : null;

			// --- Theme Management ---
			function getSystemIsDark() {
				if (origMatchMedia) {
					return origMatchMedia('(prefers-color-scheme: dark)').matches;
				}
				return true;
			}

			function applyThemeClasses(isDark) {
				var mode = isDark ? 'dark' : 'light';
				var opposite = isDark ? 'light' : 'dark';
				var root = document.documentElement;
				if (!root) return;
				root.classList.add(mode);
				root.classList.remove(opposite);
				root.setAttribute('data-theme', mode);
				root.setAttribute('data-wa-desk-theme', mode);
				root.style.colorScheme = mode;
				if (document.body) {
					document.body.classList.add(mode);
					document.body.classList.remove(opposite);
					document.body.setAttribute('data-theme', mode);
					document.body.setAttribute('data-wa-desk-theme', mode);
					document.body.style.colorScheme = mode;
				}
			}

			function ensureThemeStyle() {
				if (!themeStyle) themeStyle = document.getElementById('wa-desk-theme-style');
				if (!themeStyle) {
					themeStyle = document.createElement('style');
					themeStyle.id = 'wa-desk-theme-style';
					themeStyle.textContent = [
						'html[data-wa-desk-theme="light"], html[data-wa-desk-theme="light"] body { color-scheme: light !important; background: #f7f9fa !important; }',
						'html[data-wa-desk-theme="light"] #app, html[data-wa-desk-theme="light"] #side, html[data-wa-desk-theme="light"] #pane-side, html[data-wa-desk-theme="light"] #main { color-scheme: light !important; }'
					].join('\n');
					var target = document.head || document.documentElement;
					if (target) {
						target.appendChild(themeStyle);
					}
				}
			}

			function persistThemePreference(theme, isDark) {
				storageSet('system-theme-mode', theme === 'system' ? 'true' : 'false');
				storageSet('theme', JSON.stringify(theme === 'system' ? (isDark ? 'dark' : 'light') : theme));
				storageSet('wa-desk-theme', theme);
			}

			function scheduleThemeReapply() {
				while (themeReapplyTimers.length) clearTimeout(themeReapplyTimers.pop());
				[0, 350, 1200, 2600].forEach(function(delay) {
					themeReapplyTimers.push(setTimeout(function() {
						var isDark = currentTheme === 'system' ? getSystemIsDark() : currentTheme === 'dark';
						applyThemeClasses(isDark);
						persistThemePreference(currentTheme, isDark);
						if (window.syncToolbarBtnTheme) window.syncToolbarBtnTheme(isDark);
						if (window.syncRailSettingsBtnTheme) window.syncRailSettingsBtnTheme(isDark);
					}, delay));
				});
			}

			function applyThemeToDOM(theme) {
				currentTheme = theme;
				var isDark = (theme === 'system') ? getSystemIsDark() : (theme === 'dark');

				// 1. Update the document immediately for our controls and current page.
				applyThemeClasses(isDark);
				ensureThemeStyle();

				// 2. Synchronize WhatsApp Web's own localStorage keys before its tree settles.
				persistThemePreference(theme, isDark);

				// 3. Update modal and toolbar button if visible
				if (window.syncModalTheme) {
					window.syncModalTheme(isDark);
				}
				if (window.syncToolbarBtnTheme) {
					window.syncToolbarBtnTheme(isDark);
				}
				if (window.syncRailSettingsBtnTheme) window.syncRailSettingsBtnTheme(isDark);

				// WhatsApp may finish mounting after our script. Reapply a small, bounded
				// number of times instead of observing body classes forever: that old
				// observer could enter a feedback loop and raise CPU on Windows/macOS.
				scheduleThemeReapply();
			}

			window.getAppTheme = function() {
				return currentTheme;
			};

			window.setAppTheme = function(theme) {
				if (theme !== 'dark' && theme !== 'light' && theme !== 'system') {
					theme = 'dark';
				}
				themeChoiceVersion++;
				applyThemeToDOM(theme);
				if (window.setAppThemeNative) {
					Promise.resolve(window.setAppThemeNative(theme)).catch(function() {});
				}
				showFloatingToast(theme === 'dark' ? 'Theme: Dark' : (theme === 'light' ? 'Theme: Light' : 'Theme: System'));
				// WhatsApp keeps theme state inside its running application tree. Reload
				// once after persisting the choice so every engine starts from the same
				// localStorage state instead of leaving part of the UI in the old theme.
				clearTimeout(themeReloadTimer);
				themeReloadTimer = setTimeout(function() {
					window.location.reload();
				}, 300);
			};

			// Listen for system appearance changes
			if (origMatchMedia) {
				var sysMedia = origMatchMedia.call(window, '(prefers-color-scheme: dark)');
				var onSysChange = function() {
					if (currentTheme === 'system') {
						applyThemeToDOM('system');
						if (window.setAppThemeNative) Promise.resolve(window.setAppThemeNative('system')).catch(function() {});
					}
				};
				if (sysMedia.addEventListener) {
					sysMedia.addEventListener('change', onSysChange);
				} else if (sysMedia.addListener) {
					sysMedia.addListener(onSysChange);
				}
			}

			// Load saved theme from native settings and keep synced
			function initTheme() {
				if (themeLoadStarted || !window.getAppThemeNative) return;
				themeLoadStarted = true;
				var requestVersion = themeChoiceVersion;
				window.getAppThemeNative().then(function(savedTheme) {
					if (requestVersion !== themeChoiceVersion) return;
					if (savedTheme) applyThemeToDOM(savedTheme);
				}).catch(function() {
					themeLoadStarted = false;
				});
			}
			initTheme();
			document.addEventListener('DOMContentLoaded', function() {
				initTheme();
				applyThemeToDOM(currentTheme);
			}, { once: true });

			// --- In-Flow Header Toolbar Button (Non-Floating, Clean WhatsApp Style) ---
			function injectHeaderToolbarBtn() {
				if (document.getElementById('wa-toolbar-settings-btn')) return;

				// Target WhatsApp Web's left header above chats
				var header = window.waDOM.resolveFirst('settingsHeader', document).node;
				if (!header) return;

				// Header descendants change frequently. Use its direct trailing child, not
				// querySelector('div:last-child'), which can select an invisible nested node.
				var actionsWrap = header.lastElementChild || header;
				if (!actionsWrap) return;

				var btn = document.createElement('button');
				btn.id = 'wa-toolbar-settings-btn';
				btn.setAttribute('aria-label', 'Settings & Controls');
				btn.title = 'Settings & Controls (' + (isMac ? 'Cmd' : 'Ctrl') + ' + ,)';
				btn.style.cssText = 'width:40px;height:40px;border-radius:50%;display:inline-flex;align-items:center;justify-content:center;background:transparent;border:none;cursor:pointer;outline:none;transition:background-color 0.15s ease, color 0.15s ease;flex-shrink:0;margin:0 2px;';
				btn.innerHTML = '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">' +
					'<circle cx="12" cy="12" r="3"></circle>' +
					'<path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>' +
					'</svg>';

				// The header lives outside WhatsApp Web's own dark/light class toggling on
				// <body>, so this button previously always kept the dark-theme icon color
				// even when the app was switched to Light. Keep its resting color in sync
				// with the current app theme instead of a hardcoded dark-mode gray.
				function restingIconColor() {
					var isDarkNow = currentTheme === 'system' ?
						(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) :
						(currentTheme === 'dark');
					return isDarkNow ? '#aebac1' : '#54656f';
				}
				btn.style.color = restingIconColor();
				window.syncToolbarBtnTheme = function() {
					btn.style.color = restingIconColor();
				};

				btn.onmouseenter = function() {
					btn.style.backgroundColor = document.body.classList.contains('dark') ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)';
					btn.style.color = document.body.classList.contains('dark') ? '#e9edef' : '#111b21';
				};
				btn.onmouseleave = function() {
					btn.style.backgroundColor = 'transparent';
					btn.style.color = restingIconColor();
				};
				btn.onclick = function(e) {
					e.stopPropagation();
					window.showSettingsModal();
				};

				actionsWrap.appendChild(btn);
			}

			function isElementVisible(el) {
				if (!el || !el.isConnected) return false;
				var rect = el.getBoundingClientRect();
				if (rect.width <= 0 || rect.height <= 0) return false;
				var style = window.getComputedStyle(el);
				if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') return false;
				var docEl = document.documentElement;
				var vh = window.innerHeight || (docEl && docEl.clientHeight) || 800;
				var vw = window.innerWidth || (docEl && docEl.clientWidth) || 1200;
				if (rect.bottom <= 0 || rect.right <= 0 || rect.top >= vh || rect.left >= vw) return false;
				var p = el.parentElement;
				while (p && p !== document.body && p !== document.documentElement) {
					var ps = window.getComputedStyle(p);
					if (ps && (ps.overflow === 'hidden' || ps.overflowX === 'hidden' || ps.overflowY === 'hidden')) {
						var pr = p.getBoundingClientRect();
						if (rect.bottom <= pr.top || rect.top >= pr.bottom || rect.right <= pr.left || rect.left >= pr.right) {
							return false;
						}
					}
					p = p.parentElement;
				}
				return true;
			}

			// Keep one compact Settings control in the left rail. Header content is
			// routinely rebuilt by WhatsApp, so the rail control exists as a
			// safety net — but it must stay hidden while the header button is
			// present and visible, otherwise the user sees two identical gears.
			function ensureSettingsFallback() {
				var headerButton = document.getElementById('wa-toolbar-settings-btn');
				var headerVisible = isElementVisible(headerButton);
				var fallback = document.getElementById('wa-settings-fallback-btn');
				if (fallback) {
					fallback.setAttribute('data-header-settings-visible', headerVisible ? 'true' : 'false');
					// Hide the rail control whenever the header button works.
					fallback.style.display = headerVisible ? 'none' : 'inline-flex';
					if (window.__waRecheckEmergencySettings) window.__waRecheckEmergencySettings();
					return;
				}
				if (!document.body) return;
				fallback = document.createElement('button');
				fallback.id = 'wa-settings-fallback-btn';
				fallback.type = 'button';
				fallback.setAttribute('aria-label', 'Open Settings and Controls');
				fallback.title = 'Settings & Controls (' + (isMac ? 'Cmd' : 'Ctrl') + ' + ,)';
				fallback.innerHTML = '<svg width="21" height="21" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>';
				fallback.style.cssText = 'position:fixed;left:14px;bottom:14px;z-index:9999998;width:38px;height:38px;padding:0;display:inline-flex;align-items:center;justify-content:center;border:1px solid rgba(134,150,160,.45);border-radius:50%;background:#111b21;color:#aebac1;cursor:pointer;box-shadow:0 4px 12px rgba(0,0,0,.3);';
				fallback.setAttribute('data-header-settings-visible', headerVisible ? 'true' : 'false');
				// Only visible when the header button is missing or unusable.
				fallback.style.display = headerVisible ? 'none' : 'inline-flex';
				if (window.__waRecheckEmergencySettings) window.__waRecheckEmergencySettings();
				window.syncRailSettingsBtnTheme = function(isDark) {
					fallback.style.background = isDark ? '#111b21' : '#ffffff';
					fallback.style.color = isDark ? '#aebac1' : '#54656f';
					fallback.style.borderColor = isDark ? 'rgba(134,150,160,.45)' : 'rgba(84,101,111,.28)';
				};
				window.syncRailSettingsBtnTheme(currentTheme === 'system' ? getSystemIsDark() : currentTheme === 'dark');
				fallback.onclick = function(e) {
					e.preventDefault();
					e.stopPropagation();
					window.showSettingsModal();
				};
				document.body.appendChild(fallback);
			}

			function ensureSettingsEntryPoints() {
				injectHeaderToolbarBtn();
				// A button injected in this same tick has not been laid out yet:
				// getBoundingClientRect() is still 0x0, so isElementVisible would
				// report "hidden" and wrongly reveal the rail fallback. Re-check
				// on the next frame, when the header button has real geometry.
				requestAnimationFrame(ensureSettingsFallback);
			}

			ensureSettingsEntryPoints();
			document.addEventListener('DOMContentLoaded', function() {
				ensureSettingsEntryPoints();
				setTimeout(ensureSettingsEntryPoints, 600);
			});
			window.addEventListener('load', ensureSettingsEntryPoints);
			// WhatsApp rebuilds its header when switching chats, dropping our
			// button. Watch only #side/header region changes (rAF-coalesced)
			// instead of scanning the whole page every 2 seconds forever.
			var toolbarCheckQueued = false;
			var toolbarNarrowed = false;
			var toolbarObserver = new MutationObserver(function() {
				if (toolbarCheckQueued) return;
				toolbarCheckQueued = true;
					requestAnimationFrame(function() {
						toolbarCheckQueued = false;
						// Re-evaluate on any header change: the toolbar button may
						// have been dropped (needs re-injection) or may have become
						// hidden/clipped (rail fallback must take over). Checking the
						// visibility too is what keeps exactly one gear on screen.
						var headerButton = document.getElementById('wa-toolbar-settings-btn');
						if (!headerButton || !isElementVisible(headerButton)) {
							ensureSettingsEntryPoints();
						}
						// Narrow the observed root once the header exists.
						if (!toolbarNarrowed) {
						var hdr = window.waDOM.resolveOne('sideHeader', document).node;
							if (hdr && hdr.nodeType) {
								toolbarNarrowed = true;
								try {
									toolbarObserver.disconnect();
									toolbarObserver.observe(hdr, { childList: true, subtree: true });
								} catch (e) {}
							}
						}
					});
			});
			function watchToolbarRoot() {
				// Prefer the header itself: the chat list churns constantly and
				// never affects our button. Fall back to #side, then body, and
				// narrow down to the header as soon as it exists.
				var sideHeader = window.waDOM.resolveFirst('sideHeader', document);
				var sideRoot = window.waDOM.resolveFirst('chatListRoot', document);
				var root = sideHeader.node || sideRoot.node || document.body || document.documentElement || document;
				if (root && root.nodeType) {
					toolbarNarrowed = !!sideHeader.node;
					try {
						toolbarObserver.disconnect();
						toolbarObserver.observe(root, { childList: true, subtree: true });
					} catch (e) {}
				}
			}
			watchToolbarRoot();
			document.addEventListener('DOMContentLoaded', watchToolbarRoot, { once: true });

			// --- Minimalist WhatsApp Control Center Modal ---
			window.showSettingsModal = function() {
				if (document.getElementById('wa-settings-overlay')) {
					var ex = document.getElementById('wa-settings-overlay');
					if (ex.parentNode) ex.parentNode.removeChild(ex);
					return;
				}

				var isDark = currentTheme === 'system' ?
					(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) :
					(currentTheme === 'dark');

				var overlay = document.createElement('div');
				overlay.id = 'wa-settings-overlay';
				overlay.style.cssText = 'position:fixed;inset:0;background:rgba(8,15,19,.68);z-index:9999999;display:flex;align-items:center;justify-content:center;padding:16px;box-sizing:border-box;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;';

				var modal = document.createElement('div');
				modal.id = 'wa-settings-container';
				modal.setAttribute('role', 'dialog');
				modal.setAttribute('aria-modal', 'true');
				modal.setAttribute('aria-labelledby', 'wa-modal-title');
				modal.style.cssText = 'width:520px;max-width:96vw;max-height:90vh;border-radius:10px;box-sizing:border-box;display:flex;flex-direction:column;gap:0;overflow-y:auto;padding:0 22px 18px;box-shadow:0 18px 48px rgba(0,0,0,.32);';

				// Header
				var header = document.createElement('div');
				header.id = 'wa-modal-header';
				header.style.cssText = 'display:flex;align-items:center;justify-content:space-between;border-bottom-width:1px;border-bottom-style:solid;padding:18px 0 14px;margin-bottom:2px;';
				header.innerHTML = '' +
					'<div style="display:flex;align-items:center;gap:10px;">' +
					'  <div id="wa-modal-icon-wrap" style="width:10px;height:10px;border-radius:50%;display:flex;align-items:center;justify-content:center;background:#00a884;">' +
					'  </div>' +
					'  <div>' +
					'    <h3 id="wa-modal-title" style="margin:0;font-size:15px;font-weight:600;">WhatsApp Desk</h3>' +
					'    <span id="wa-modal-sub" style="font-size:11px;">Application settings · version __WA_APP_VERSION__</span>' +
					'  </div>' +
					'</div>' +
					'<button id="wa-settings-close-x" aria-label="Close Control Center" style="background:transparent;border:none;cursor:pointer;font-size:18px;line-height:1;padding:4px 8px;border-radius:4px;">✕</button>';
				modal.appendChild(header);

				// Keyboard support: Escape closes (existing keydown), Tab is
				// trapped inside the dialog, and focus starts on the close
				// button so keyboard users enter the dialog predictably.
				var previouslyFocused = document.activeElement;
				modal.addEventListener('keydown', function(e) {
					if (e.key !== 'Tab') return;
					var focusables = modal.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
					if (!focusables.length) return;
					var first = focusables[0];
					var last = focusables[focusables.length - 1];
					if (e.shiftKey && document.activeElement === first) {
						e.preventDefault();
						last.focus();
					} else if (!e.shiftKey && document.activeElement === last) {
						e.preventDefault();
						first.focus();
					}
				});
				var closeModalAndRestore = (typeof closeSettings === 'function') ? closeSettings : null;
				overlay.addEventListener('keydown', function(e) {
					if (e.key === 'Escape' && closeModalAndRestore) closeModalAndRestore();
				});

				// Section 0: Theme Switcher Segmented Control
				var themeBox = document.createElement('div');
				themeBox.className = 'wa-modal-card';
				themeBox.style.cssText = 'display:flex;align-items:center;justify-content:space-between;padding:14px 0;border-radius:0;border-width:0 0 1px;border-style:solid;gap:16px;';
				themeBox.innerHTML = '' +
					'<div>' +
					'  <strong class="wa-text-primary" style="font-size:12.5px;display:block;">Appearance</strong>' +
					'  <span class="wa-text-muted" style="font-size:11px;">Application interface theme</span>' +
					'</div>' +
					'<div style="display:flex;align-items:center;gap:4px;">' +
					'  <button id="wa-theme-btn-dark" class="wa-theme-btn" style="padding:5px 10px;border-radius:6px;font-size:11.5px;cursor:pointer;border-width:1px;border-style:solid;font-weight:500;">Dark</button>' +
					'  <button id="wa-theme-btn-light" class="wa-theme-btn" style="padding:5px 10px;border-radius:6px;font-size:11.5px;cursor:pointer;border-width:1px;border-style:solid;font-weight:500;">Light</button>' +
					'  <button id="wa-theme-btn-system" class="wa-theme-btn" style="padding:5px 10px;border-radius:6px;font-size:11.5px;cursor:pointer;border-width:1px;border-style:solid;font-weight:500;">System</button>' +
					'</div>';
				modal.appendChild(themeBox);

				// Section 1: Quick Interactive Controls (2-Column Grid)
				var quickGrid = document.createElement('div');
				quickGrid.style.cssText = 'display:flex;flex-direction:column;';

				if (isMac && window.getCameraPermissionNative && window.getMicrophonePermissionNative) {
					mediaPermissionCard = document.createElement('div');
					mediaPermissionCard.className = 'wa-modal-card';
					mediaPermissionCard.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;flex-direction:column;gap:8px;';
					mediaPermissionCard.innerHTML = '' +
						'<div style="display:flex;align-items:center;justify-content:space-between;gap:16px;">' +
						'  <div>' +
						'    <strong class="wa-text-primary" style="font-size:12.5px;display:block;">Camera & Microphone</strong>' +
						'    <span id="wa-media-permission-summary" class="wa-text-muted" style="font-size:11px;display:block;margin-top:2px;">Checking macOS permissions...</span>' +
						'  </div>' +
						'  <div style="display:flex;gap:6px;flex-shrink:0;">' +
						'    <button id="wa-media-permission-settings" class="wa-card-btn" style="padding:4px 8px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">System Settings</button>' +
						'    <button id="wa-media-permission-retry" class="wa-card-btn" style="padding:4px 8px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Retry</button>' +
						'  </div>' +
						'</div>' +
						'<div id="wa-media-permission-details" class="wa-text-muted" style="font-size:10.5px;line-height:1.45;"></div>';
					quickGrid.appendChild(mediaPermissionCard);
				}

				// Card 1: Desktop Notifications
				var cardNotifications = document.createElement('div');
				cardNotifications.className = 'wa-modal-card';
				cardNotifications.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;align-items:center;justify-content:space-between;gap:16px;';
				cardNotifications.innerHTML = '' +
					'<div>' +
					'  <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:2px;">' +
					'    <strong class="wa-text-primary" style="font-size:12.5px;">Desktop Notifications</strong>' +
					'    <span id="wa-badge-notifications" style="font-size:10px;padding:1px 5px;border-radius:4px;font-weight:600;">...</span>' +
					'  </div>' +
					'  <div class="wa-text-muted" style="font-size:11px;">Allow or block system notifications on macOS, Linux, and Windows.</div>' +
					'</div>' +
					'<button id="wa-action-toggle-notifications" class="wa-card-btn" style="flex-shrink:0;min-width:78px;padding:4px 10px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Disable</button>';
				quickGrid.appendChild(cardNotifications);

				// Card: privacy profiles and native app lock.
				var cardPrivacy = document.createElement('div');
				cardPrivacy.className = 'wa-modal-card';
				cardPrivacy.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;flex-direction:column;gap:10px;';
				cardPrivacy.innerHTML = '' +
					'<div style="display:flex;align-items:center;justify-content:space-between;gap:16px;">' +
					'  <div><strong class="wa-text-primary" style="font-size:12.5px;display:block;">Privacy profiles</strong>' +
					'  <span class="wa-text-muted" style="font-size:11px;">Choose which WhatsApp surfaces are blurred. Visual blur does not lock the account or session.</span></div>' +
					'  <div style="display:flex;align-items:center;gap:8px;flex-shrink:0;">' +
					'    <span class="wa-text-muted" style="font-size:10px;font-family:monospace;">' + (isMac ? 'Cmd' : 'Ctrl') + '+Shift+P</span>' +
					'    <button id="wa-action-toggle-priv" class="wa-card-btn" style="min-width:78px;padding:4px 10px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Toggle all</button>' +
					'  </div>' +
					'</div>' +
					'<label class="wa-text-muted" style="font-size:11px;display:flex;align-items:center;gap:8px;">Active profile' +
					'  <select id="wa-privacy-profile" aria-label="Active privacy profile" style="min-width:190px;padding:5px;border-radius:6px;">' +
					'    <option value="normal">Normal</option><option value="office">Office</option><option value="presentation">Presentation</option><option value="maximum-privacy">Maximum Privacy</option><option value="custom">Custom</option>' +
					'  </select>' +
					'</label>' +
					'<div id="wa-privacy-surface-controls" style="display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:6px 12px;"></div>' +
					'<div style="display:flex;align-items:center;gap:10px;flex-wrap:wrap;">' +
					'  <label class="wa-text-muted" style="font-size:11px;">Reveal <select id="wa-privacy-reveal-mode" aria-label="Privacy reveal method" style="padding:4px;border-radius:5px;"><option value="hover">Hover or keyboard focus</option><option value="click">Click or keyboard focus</option><option value="modifier">Hold modifier key</option></select></label>' +
					'  <label class="wa-text-muted" style="font-size:11px;">Key <select id="wa-privacy-reveal-modifier" aria-label="Privacy reveal modifier" style="padding:4px;border-radius:5px;"><option value="alt">Alt</option><option value="shift">Shift</option><option value="ctrl">Ctrl</option></select></label>' +
					'  <button id="wa-privacy-copy-custom" class="wa-card-btn" style="padding:4px 8px;border-radius:6px;font-size:11px;cursor:pointer;border-width:1px;border-style:solid;">Copy to Custom</button>' +
					'  <button id="wa-privacy-reset" class="wa-card-btn" style="padding:4px 8px;border-radius:6px;font-size:11px;cursor:pointer;border-width:1px;border-style:solid;">Reset built-in profiles</button>' +
					'</div>' +
					'<div style="display:flex;align-items:center;justify-content:space-between;gap:10px;border-top:1px solid rgba(127,127,127,.25);padding-top:10px;">' +
					'  <div><strong class="wa-text-primary" style="font-size:12px;display:block;">App lock</strong><span id="wa-lock-status" class="wa-text-muted" style="font-size:11px;">Checking lock settings…</span></div>' +
					'  <div style="display:flex;gap:6px;"><button id="wa-lock-now" class="wa-card-btn" style="padding:4px 8px;border-radius:6px;font-size:11px;cursor:pointer;border-width:1px;border-style:solid;">Lock now</button><button id="wa-lock-manage" class="wa-card-btn" style="padding:4px 8px;border-radius:6px;font-size:11px;cursor:pointer;border-width:1px;border-style:solid;">Manage lock</button></div>' +
					'</div>';
				quickGrid.appendChild(cardPrivacy);

				// Card 2: Always on Top
				var cardPin = document.createElement('div');
				cardPin.className = 'wa-modal-card';
				cardPin.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;align-items:center;justify-content:space-between;gap:16px;';
				cardPin.innerHTML = '' +
					'<div style="flex:1;min-width:0;">' +
					'  <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:2px;">' +
					'    <strong class="wa-text-primary" style="font-size:12.5px;">Always on Top</strong>' +
					'    <span id="wa-badge-pin" style="font-size:10px;padding:1px 5px;border-radius:4px;font-weight:600;">...</span>' +
					'  </div>' +
					'  <div class="wa-text-muted" style="font-size:11px;">Keep window floating above other applications.</div>' +
					'</div>' +
					'<div style="display:flex;align-items:center;justify-content:flex-end;gap:12px;min-width:150px;flex-shrink:0;">' +
					'  <span class="wa-text-muted" style="font-size:10px;font-family:monospace;">' + (isMac ? 'Cmd' : 'Ctrl') + '+Shift+T</span>' +
					'  <button id="wa-action-toggle-pin" class="wa-card-btn" style="min-width:78px;padding:4px 10px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Toggle</button>' +
					'</div>';
				quickGrid.appendChild(cardPin);

				// Card 3: Audio Mute
				var cardMute = document.createElement('div');
				cardMute.className = 'wa-modal-card';
				cardMute.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;align-items:center;justify-content:space-between;gap:16px;';
				cardMute.innerHTML = '' +
					'<div style="flex:1;min-width:0;">' +
					'  <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:2px;">' +
					'    <strong class="wa-text-primary" style="font-size:12.5px;">Notification Audio</strong>' +
					'    <span id="wa-badge-mute" style="font-size:10px;padding:1px 5px;border-radius:4px;font-weight:600;">...</span>' +
					'  </div>' +
					'  <div class="wa-text-muted" style="font-size:11px;">Mute all notification sounds and media audio.</div>' +
					'</div>' +
					'<div style="display:flex;align-items:center;justify-content:flex-end;gap:12px;min-width:150px;flex-shrink:0;">' +
					'  <span class="wa-text-muted" style="font-size:10px;font-family:monospace;">' + (isMac ? 'Cmd' : 'Ctrl') + '+Shift+M</span>' +
					'  <button id="wa-action-toggle-mute" class="wa-card-btn" style="min-width:78px;padding:4px 10px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Toggle</button>' +
					'</div>';
				quickGrid.appendChild(cardMute);

				// Card 4: Auto-Start
				var cardAuto = document.createElement('div');
				cardAuto.className = 'wa-modal-card';
				cardAuto.style.cssText = 'border-radius:0;border-width:0 0 1px;border-style:solid;padding:12px 0;display:flex;align-items:center;justify-content:space-between;gap:16px;';
				cardAuto.innerHTML = '' +
					'<div style="flex:1;min-width:0;">' +
					'  <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:2px;">' +
					'    <strong class="wa-text-primary" style="font-size:12.5px;">Launch at Startup</strong>' +
					'    <span id="wa-badge-auto" style="font-size:10px;padding:1px 5px;border-radius:4px;font-weight:600;">...</span>' +
					'  </div>' +
					'  <div class="wa-text-muted" style="font-size:11px;">Automatically start WhatsApp Desk on system login.</div>' +
					'</div>' +
					'<div style="display:flex;align-items:center;justify-content:flex-end;gap:12px;min-width:150px;flex-shrink:0;">' +
					'  <span class="wa-text-muted" style="font-size:10px;font-family:monospace;">' + (isMac ? 'Cmd' : 'Ctrl') + '+Shift+S</span>' +
					'  <button id="wa-action-toggle-auto" class="wa-card-btn" style="min-width:78px;padding:4px 10px;border-radius:6px;font-size:11px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;">Toggle</button>' +
					'</div>';
				quickGrid.appendChild(cardAuto);

				modal.appendChild(quickGrid);

				// Section 2: Download Folder Settings
				var folderSection = document.createElement('div');
				folderSection.className = 'wa-modal-card';
				folderSection.style.cssText = 'display:flex;flex-direction:column;gap:8px;border-radius:0;border-width:0 0 1px;border-style:solid;padding:14px 0;';
				folderSection.innerHTML = '' +
					'<div style="display:flex;align-items:center;justify-content:space-between;">' +
					'  <strong class="wa-text-primary" style="font-size:12.5px;">Downloads folder</strong>' +
					'  <button id="wa-btn-reset-folder" style="background:transparent;border:none;color:#00a884;font-size:11px;cursor:pointer;padding:2px 4px;">Use default</button>' +
					'</div>' +
					'<div class="wa-text-muted" style="font-size:11px;">Files & media downloaded from chat are permanently saved here:</div>' +
					'<div id="wa-folder-box" style="display:flex;align-items:center;border-width:1px;border-style:solid;border-radius:6px;padding:6px 8px;min-width:0;">' +
					'  <span id="wa-folder-path" style="font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;flex:1;font-family:monospace;">Loading...</span>' +
					'</div>' +
					'<div style="display:flex;align-items:center;gap:6px;margin-top:2px;">' +
					'  <button id="wa-btn-change-folder" class="wa-card-btn" style="flex:1;padding:6px 10px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;border-width:1px;border-style:solid;">Change Folder Location...</button>' +
					'  <button id="wa-btn-open-folder" style="background:#00a884;color:#111b21;border:none;padding:6px 12px;border-radius:6px;font-size:11.5px;font-weight:600;cursor:pointer;">' + (isMac ? 'Open in Finder' : 'Open Folder') + '</button>' +
					'</div>' +
					'<label style="display:flex;align-items:center;gap:8px;cursor:pointer;user-select:none;margin-top:2px;">' +
					'  <input type="checkbox" id="wa-organize-month" style="width:14px;height:14px;accent-color:#00a884;cursor:pointer;margin:0;" />' +
					'  <span class="wa-text-muted" style="font-size:11px;">Organize into monthly subfolders (2026-09)</span>' +
					'</label>';
				modal.appendChild(folderSection);

				// Section 3: Maintenance & Update Actions
				var actionsSection = document.createElement('div');
				actionsSection.className = 'wa-modal-card';
				actionsSection.style.cssText = 'display:flex;flex-direction:column;gap:8px;border-radius:0;border-width:0 0 1px;border-style:solid;padding:14px 0;';
				actionsSection.innerHTML = '' +
					'<strong class="wa-text-primary" style="font-size:12.5px;">Maintenance</strong>' +
					'<div style="display:grid;grid-template-columns:1fr 1fr;gap:6px;">' +
					'  <button id="wa-btn-check-updates-modal" class="wa-card-btn" style="padding:6px 8px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;border-width:1px;border-style:solid;text-align:center;">Check for updates</button>' +
					'  <button id="wa-btn-reload-modal" class="wa-card-btn" style="padding:6px 8px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;border-width:1px;border-style:solid;text-align:center;">Reload chat</button>' +
					'  <button id="wa-btn-hardref-modal" class="wa-card-btn" style="padding:6px 8px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;border-width:1px;border-style:solid;text-align:center;">Clear cache</button>' +
					'  <button id="wa-btn-onboard-modal" class="wa-card-btn" style="padding:6px 8px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;border-width:1px;border-style:solid;text-align:center;">View welcome guide</button>' +
					'</div>';
				modal.appendChild(actionsSection);

				// Section 4: Help & local diagnostics. This intentionally performs no
				// network request and never reads chat data; it only validates the
				// small native bridge surface used by the application.
				var helpSection = document.createElement('div');
				helpSection.className = 'wa-modal-card';
				helpSection.style.cssText = 'display:flex;flex-direction:column;gap:8px;border-radius:0;border-width:0 0 1px;border-style:solid;padding:14px 0;';
				helpSection.innerHTML = '' +
					'<div style="display:flex;align-items:center;justify-content:space-between;gap:12px;">' +
					'  <div><strong class="wa-text-primary" style="font-size:12.5px;">Help & diagnostics</strong><div class="wa-text-muted" style="font-size:11px;margin-top:2px;">Check the app surface locally. No chats or files are sent.</div></div>' +
					'  <button id="wa-btn-run-diagnostics" class="wa-card-btn" style="padding:6px 10px;border-radius:6px;font-size:11.5px;font-weight:500;cursor:pointer;border-width:1px;border-style:solid;white-space:nowrap;">Run quick check</button>' +
					'</div>' +
					'<div id="wa-diagnostics-result" class="wa-text-muted" aria-live="polite" style="display:none;font-size:10.5px;line-height:1.45;border-radius:6px;padding:7px 8px;"></div>' +
					'<button id="wa-btn-show-shortcuts" style="align-self:flex-start;background:transparent;border:none;color:#00a884;font-size:11px;cursor:pointer;padding:2px 0;">View keyboard shortcuts</button>' +
					'<div id="wa-shortcuts-list" class="wa-text-muted" style="display:none;font-size:10.5px;line-height:1.65;"></div>';
				modal.appendChild(helpSection);

				// Disclaimer
				var disclaimer = document.createElement('div');
				disclaimer.className = 'wa-text-muted';
				disclaimer.style.cssText = 'font-size:10px;line-height:1.4;border-top-width:1px;border-top-style:solid;padding-top:8px;margin-top:2px;';
				disclaimer.innerHTML = '<strong>WhatsApp Desk</strong> is an independent application and is not affiliated with Meta.';
				modal.appendChild(disclaimer);

				// Footer
				var footer = document.createElement('div');
				footer.style.cssText = 'display:flex;justify-content:space-between;align-items:center;margin-top:2px;';
				footer.innerHTML = '<span class="wa-text-muted" style="font-size:10.5px;">Press <kbd style="padding:1px 3px;border-radius:3px;font-family:monospace;">Esc</kbd> to close</span>';
				var footLeft = document.createElement('div');
				footLeft.style.cssText = 'display:flex;align-items:center;gap:8px;';
				var btnReport = document.createElement('button');
				btnReport.textContent = '🐞 Report issue';
				btnReport.id = 'wa-btn-report';
				btnReport.title = 'Open a pre-filled GitHub issue with recent errors (nothing is sent automatically)';
				btnReport.style.cssText = 'background:transparent;border:none;color:#8696a0;font-size:11px;cursor:pointer;padding:5px 8px;';
				btnReport.onclick = function() {
					closeSettings();
					if (window.reportIssueNow) window.reportIssueNow();
				};
				var btnDone = document.createElement('button');
				btnDone.textContent = 'Done';
				btnDone.id = 'wa-btn-done';
				btnDone.style.cssText = 'padding:5px 16px;border-radius:6px;font-size:11.5px;font-weight:600;cursor:pointer;border-width:1px;border-style:solid;';
				footLeft.appendChild(btnReport);
				footLeft.appendChild(btnDone);
				footer.appendChild(footLeft);
				modal.appendChild(footer);

				overlay.appendChild(modal);
				document.body.appendChild(overlay);
				var closeXForFocus = document.getElementById('wa-settings-close-x');
				if (closeXForFocus) closeXForFocus.focus();
				modal.addEventListener('pointerdown', function(e) { e.stopPropagation(); });
				modal.addEventListener('click', function(e) { e.stopPropagation(); });

				function closeSettings() {
					window.removeEventListener('keydown', onKeyClose);
					window.syncModalTheme = null;
					if (overlay.parentNode) overlay.parentNode.removeChild(overlay);
				}
				function onKeyClose(e) {
					if (e.key === 'Escape') closeSettings();
				}
				window.addEventListener('keydown', onKeyClose);
				btnDone.onclick = closeSettings;
				document.getElementById('wa-settings-close-x').onclick = closeSettings;
				overlay.onclick = function(e) {
					if (e.target === overlay) closeSettings();
				};

				// Styling Synchronizer for Modal (Dark / Light Theme)
				window.syncModalTheme = function(isThemeDark) {
					var bg = isThemeDark ? '#111b21' : '#ffffff';
					var cardBg = bg;
					var border = isThemeDark ? '#2a3942' : '#d1d7db';
					var textPri = isThemeDark ? '#e9edef' : '#111b21';
					var textMut = isThemeDark ? '#8696a0' : '#667781';
					var accent = isThemeDark ? '#00a884' : '#008069';

					modal.style.background = bg;
					modal.style.border = '1px solid ' + border;
					header.style.borderBottomColor = border;
					document.getElementById('wa-modal-title').style.color = textPri;
					document.getElementById('wa-modal-sub').style.color = textMut;
					document.getElementById('wa-modal-icon-wrap').style.background = accent;
					document.getElementById('wa-modal-icon-wrap').style.color = accent;
					document.getElementById('wa-settings-close-x').style.color = textMut;

					document.querySelectorAll('.wa-modal-card').forEach(function(el) {
						el.style.background = cardBg;
						el.style.borderColor = border;
					});
					document.querySelectorAll('.wa-text-primary').forEach(function(el) {
						el.style.color = textPri;
					});
					document.querySelectorAll('.wa-text-muted').forEach(function(el) {
						el.style.color = textMut;
					});

					var fBox = document.getElementById('wa-folder-box');
					if (fBox) {
						fBox.style.background = isThemeDark ? '#111b21' : '#ffffff';
						fBox.style.borderColor = border;
					}
					var fPath = document.getElementById('wa-folder-path');
					if (fPath) fPath.style.color = textMut;

					var btnOpen = document.getElementById('wa-btn-open-folder');
					if (btnOpen) {
						btnOpen.style.background = accent;
						btnOpen.style.color = isThemeDark ? '#111b21' : '#ffffff';
					}

					document.querySelectorAll('.wa-card-btn').forEach(function(el) {
						el.style.background = isThemeDark ? '#111b21' : '#ffffff';
						el.style.borderColor = border;
						el.style.color = textPri;
					});

					btnDone.style.background = isThemeDark ? '#202c33' : '#e9edef';
					btnDone.style.borderColor = border;
					btnDone.style.color = textPri;

					// Theme segment buttons
					['dark', 'light', 'system'].forEach(function(mode) {
						var tBtn = document.getElementById('wa-theme-btn-' + mode);
						if (tBtn) {
							var active = (currentTheme === mode);
							tBtn.style.background = active ? accent : (isThemeDark ? '#111b21' : '#ffffff');
							tBtn.style.color = active ? (isThemeDark ? '#111b21' : '#ffffff') : textPri;
							tBtn.style.borderColor = active ? accent : border;
						}
					});
				};

				// Synchronize Toggle Badges & Button States
				function updateBadges() {
					var isThemeDark = currentTheme === 'system' ?
						(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) :
						(currentTheme === 'dark');
					var accent = isThemeDark ? '#00a884' : '#008069';

					var privActive = window.isPrivacyModeActive ? window.isPrivacyModeActive() : false;
					var notificationActive = window.isNotificationsEnabled ? window.isNotificationsEnabled() : true;
					var badgeNotifications = document.getElementById('wa-badge-notifications');
					var btnNotifications = document.getElementById('wa-action-toggle-notifications');
					if (badgeNotifications && btnNotifications) {
						badgeNotifications.textContent = notificationActive ? 'Enabled' : 'Disabled';
						badgeNotifications.style.background = notificationActive ? (isThemeDark ? 'rgba(0,168,132,0.15)' : 'rgba(0,128,105,0.15)') : 'transparent';
						badgeNotifications.style.color = notificationActive ? accent : '#8696a0';
						btnNotifications.textContent = notificationActive ? 'Disable' : 'Enable';
					}
					var badgePriv = document.getElementById('wa-badge-priv');
					var btnPriv = document.getElementById('wa-action-toggle-priv');
					if (badgePriv && btnPriv) {
						badgePriv.textContent = privActive ? 'Enabled' : 'Disabled';
						badgePriv.style.background = privActive ? (isThemeDark ? 'rgba(0,168,132,0.15)' : 'rgba(0,128,105,0.15)') : 'transparent';
						badgePriv.style.color = privActive ? accent : '#8696a0';
						btnPriv.textContent = privActive ? 'Disable' : 'Enable';
					}

					var pinActive = window.isAlwaysOnTopActive ? window.isAlwaysOnTopActive() : false;
					var badgePin = document.getElementById('wa-badge-pin');
					var btnPin = document.getElementById('wa-action-toggle-pin');
					if (badgePin && btnPin) {
						badgePin.textContent = pinActive ? 'Pinned' : 'Unpinned';
						badgePin.style.background = pinActive ? (isThemeDark ? 'rgba(0,168,132,0.15)' : 'rgba(0,128,105,0.15)') : 'transparent';
						badgePin.style.color = pinActive ? accent : '#8696a0';
						btnPin.textContent = pinActive ? 'Unpin' : 'Pin';
					}

					var muteActive = window.isAudioMuted ? window.isAudioMuted() : false;
					var badgeMute = document.getElementById('wa-badge-mute');
					var btnMute = document.getElementById('wa-action-toggle-mute');
					if (badgeMute && btnMute) {
						badgeMute.textContent = muteActive ? 'Muted' : 'Unmuted';
						badgeMute.style.background = muteActive ? 'rgba(234,0,56,0.15)' : 'transparent';
						badgeMute.style.color = muteActive ? '#ff5252' : accent;
						btnMute.textContent = muteActive ? 'Unmute' : 'Mute';
					}

					var autoActive = window.isAutoStartActive ? window.isAutoStartActive() : false;
					var badgeAuto = document.getElementById('wa-badge-auto');
					var btnAuto = document.getElementById('wa-action-toggle-auto');
					if (badgeAuto && btnAuto) {
						badgeAuto.textContent = autoActive ? 'Enabled' : 'Disabled';
						badgeAuto.style.background = autoActive ? (isThemeDark ? 'rgba(0,168,132,0.15)' : 'rgba(0,128,105,0.15)') : 'transparent';
						badgeAuto.style.color = autoActive ? accent : '#8696a0';
						btnAuto.textContent = autoActive ? 'Disable' : 'Enable';
					}

					window.syncModalTheme(isThemeDark);
				}
				function mediaPermissionText(status) {
					if (status === 'authorized') return 'Allowed';
					if (status === 'denied') return 'Blocked';
					if (status === 'restricted') return 'Restricted';
					return 'Not requested';
				}

				function refreshMediaPermissions() {
					if (!mediaPermissionCard) return Promise.resolve();
					var summary = document.getElementById('wa-media-permission-summary');
					var details = document.getElementById('wa-media-permission-details');
					return Promise.all([
						Promise.resolve(window.getCameraPermissionNative()).catch(function() { return 'unknown'; }),
						Promise.resolve(window.getMicrophonePermissionNative()).catch(function() { return 'unknown'; })
					]).then(function(statuses) {
						var camera = statuses[0];
						var microphone = statuses[1];
						var blocked = camera === 'denied' || camera === 'restricted' || microphone === 'denied' || microphone === 'restricted';
						if (summary) summary.textContent = 'Camera: ' + mediaPermissionText(camera) + ' · Microphone: ' + mediaPermissionText(microphone);
						if (details) details.textContent = blocked ?
							'Permission is blocked by macOS. Open System Settings → Privacy & Security → Camera/Microphone, enable WhatsApp Desk, then click Retry.' :
							'WhatsApp Desk needs these permissions for calls and voice messages. If macOS asks, allow access and click Retry.';
					});
				}

				if (mediaPermissionCard) {
					document.getElementById('wa-media-permission-settings').onclick = function() {
						if (window.openMediaPrivacySettingsNative) window.openMediaPrivacySettingsNative('camera');
					};
					document.getElementById('wa-media-permission-retry').onclick = function() {
						var retry = document.getElementById('wa-media-permission-retry');
						if (retry) { retry.disabled = true; retry.textContent = 'Checking...'; }
						var request = navigator.mediaDevices && navigator.mediaDevices.getUserMedia ?
							navigator.mediaDevices.getUserMedia({ audio: true, video: true }).then(function(stream) {
								stream.getTracks().forEach(function(track) { track.stop(); });
							}).catch(function() {}) : Promise.resolve();
						request.then(refreshMediaPermissions).then(function() {
							if (retry) { retry.disabled = false; retry.textContent = 'Retry'; }
						});
					};
					refreshMediaPermissions();
				}

				document.getElementById('wa-action-toggle-notifications').onclick = function() {
					if (window.setNotificationsEnabled) {
						window.setNotificationsEnabled(!window.isNotificationsEnabled()).then(function() {
							updateBadges();
							showFloatingToast(window.isNotificationsEnabled() ? '🔔 Desktop notifications enabled' : '🔕 Desktop notifications disabled');
						});
					}
				};
				if (window.refreshNotificationsEnabled) {
					window.refreshNotificationsEnabled().then(function() { updateBadges(); });
				}
				updateBadges();
				if (window.refreshAutoStartState) {
					window.refreshAutoStartState().then(function() { updateBadges(); });
				}

				// Hook Theme Segmented Control
				document.getElementById('wa-theme-btn-dark').onclick = function() {
					window.setAppTheme('dark');
					updateBadges();
				};
				document.getElementById('wa-theme-btn-light').onclick = function() {
					window.setAppTheme('light');
					updateBadges();
				};
				document.getElementById('wa-theme-btn-system').onclick = function() {
					window.setAppTheme('system');
					updateBadges();
				};

				// Hook Click Actions
				document.getElementById('wa-action-toggle-priv').onclick = function() {
					if (window.togglePrivacyMode) window.togglePrivacyMode();
					updateBadges();
				};
				var profileSelect = document.getElementById('wa-privacy-profile');
				var privacyControls = document.getElementById('wa-privacy-surface-controls');
				var privacyFields = [
					['chat_names', 'Chat names'], ['group_names', 'Group names'], ['avatars', 'Avatars'],
					['preview', 'Message previews'], ['timestamps', 'Timestamps'], ['unread_count', 'Unread counts'],
					['message_text', 'Message text'], ['images', 'Images'], ['videos', 'Videos'], ['stickers', 'Stickers'],
					['quoted_content', 'Quoted or replied content'], ['voice_note_details', 'Voice-note details'],
					['header_name', 'Conversation name'], ['header_avatar', 'Conversation avatar'],
					['header_subtitle', 'Conversation subtitle'], ['media_viewer', 'Media viewer']
				];
				function refreshPrivacyCard() {
					if (!window.getPrivacyProfiles || !privacyControls) return;
					var currentID = window.getActivePrivacyProfile ? window.getActivePrivacyProfile() : 'normal';
					var profiles = window.getPrivacyProfiles();
					var active = profiles.filter(function(profile) { return profile.id === currentID; })[0];
					var supported = !!window.getPrivacyProfileStateNative;
					profileSelect.value = currentID;
					profileSelect.disabled = !supported;
					privacyControls.innerHTML = '';
					privacyFields.forEach(function(field) {
						var label = document.createElement('label');
						label.style.cssText = 'display:flex;align-items:center;gap:7px;cursor:pointer;user-select:none;font-size:11px;';
						var input = document.createElement('input');
						input.type = 'checkbox';
						input.checked = !!(active && active.privacy && active.privacy[field[0]]);
						input.disabled = !supported || currentID !== 'custom';
						input.setAttribute('aria-label', field[1]);
						input.onchange = function() {
							if (window.setCustomPrivacySurface) window.setCustomPrivacySurface(field[0], input.checked).then(refreshPrivacyCard);
						};
						var text = document.createElement('span');
						text.textContent = field[1];
						label.appendChild(input);
						label.appendChild(text);
						privacyControls.appendChild(label);
					});
					var revealMode = document.getElementById('wa-privacy-reveal-mode');
					var revealModifier = document.getElementById('wa-privacy-reveal-modifier');
					var editable = supported && currentID === 'custom';
					if (active && active.privacy) {
						revealMode.value = active.privacy.reveal_mode || 'hover';
						revealModifier.value = active.privacy.reveal_modifier || 'alt';
					}
					revealMode.disabled = !editable;
					revealModifier.disabled = !editable;
					document.getElementById('wa-privacy-copy-custom').disabled = !supported;
					document.getElementById('wa-privacy-reset').disabled = !supported;
					// Local pins (M5-02): render from the native store; identity
					// resolution and opening go through the DOM adapter.
					var pinsList = document.getElementById('wa-pins-list');
					var pinsStatus = document.getElementById('wa-pins-status');
					var renderPins = function() {
						if (!pinsList) return;
						window.waPinList().then(function(envelope) {
							pinsList.innerHTML = '';
							var pins = (envelope && envelope.ok && envelope.pins) ? envelope.pins : [];
							pinsStatus.textContent = pins.length === 0 ? 'No pins yet. Open a chat list row, then pin the active chat.' : pins.length + ' pin(s), stored locally only.';
							pins.forEach(function(pin, index) {
								if (!matchesFilter(pin.display_label || 'Pinned chat')) return;
								var row = document.createElement('div');
								row.style.cssText = 'display:flex;align-items:center;gap:6px;font-size:11px;';
								var label = document.createElement('span');
								label.className = 'wa-text-primary';
								label.style.cssText = 'flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;';
								label.textContent = pin.display_label || 'Pinned chat';
								label.title = pin.confidence === 'high' ? 'High-confidence identity' : 'Medium-confidence identity (title-based; a rename may invalidate it)';
								var chip = document.createElement('span');
								chip.style.cssText = 'font-size:9px;padding:1px 4px;border-radius:4px;border:1px solid rgba(127,127,127,.35);flex-shrink:0;';
								chip.textContent = pin.confidence === 'high' ? 'high' : 'medium';
								var mkBtn = function(text, title, handler) {
									var button = document.createElement('button');
									button.className = 'wa-card-btn';
									button.style.cssText = 'padding:2px 6px;border-radius:5px;font-size:10px;cursor:pointer;border-width:1px;border-style:solid;flex-shrink:0;';
									button.textContent = text;
									button.title = title;
									button.onclick = handler;
									return button;
								};
								var move = function(delta) {
									var ids = pins.map(function(p) { return p.ID; });
									var target = index + delta;
									if (target < 0 || target >= ids.length) return;
									var swapped = ids[index];
									ids[index] = ids[target];
									ids[target] = swapped;
									window.waPinReorder(ids).then(renderPins);
								};
								var doOpen = function() {
									window.waPinOpen(pin.chat_key).then(function(result) {
										if (!result.ok) window.showFloatingToast && window.showFloatingToast('📌 ' + result.error);
									});
								};
								var doRename = function() {
									if (label.querySelector('input')) return;
									var input = document.createElement('input');
									input.value = pin.display_label || '';
									input.maxLength = 128;
									input.setAttribute('aria-label', 'Pin alias');
									input.style.cssText = 'flex:1;min-width:0;padding:2px 4px;font-size:11px;';
									label.textContent = '';
									label.appendChild(input);
									input.focus();
									var commit = function() { window.waPinRename(pin.ID, input.value).then(renderPins); };
									input.onkeydown = function(e) { if (e.key === 'Enter') commit(); if (e.key === 'Escape') renderPins(); };
									input.onblur = commit;
								};
								row.appendChild(chip);
								row.appendChild(label);
								row.appendChild(mkBtn('Open', 'Open this pinned chat', doOpen));
								row.appendChild(mkBtn('↑', 'Move up', function() { move(-1); }));
								row.appendChild(mkBtn('↓', 'Move down', function() { move(1); }));
								row.appendChild(mkBtn('Rename', 'Rename pin', doRename));
								row.appendChild(mkBtn('✕', 'Remove pin', function() { window.waPinRemove(pin.ID).then(renderPins); }));
								pinsList.appendChild(row);
							});
						}).catch(function() { pinsStatus.textContent = 'Pins unavailable'; });
					};
					var pinAdd = document.getElementById('wa-pin-add');
					if (pinAdd) pinAdd.onclick = function() {
						window.waPinActiveChat().then(function(result) {
							if (result.ok) renderPins();
							else window.showFloatingToast && window.showFloatingToast('📌 ' + (result.error || 'Could not pin this chat'));
						});
					};
					var pinClear = document.getElementById('wa-pin-clear');
					if (pinClear) pinClear.onclick = function() {
						window.waPinClear().then(renderPins);
					};
					renderPins();

					// Local bookmarks (M5-03): render from the native store; the
					// list never contains message bodies, only identity metadata.
					var bookmarksList = document.getElementById('wa-bookmarks-list');
					var bookmarksStatus = document.getElementById('wa-bookmarks-status');
					var renderBookmarks = function() {
						if (!bookmarksList) return;
						window.waBookmarkList().then(function(envelope) {
							bookmarksList.innerHTML = '';
							var bookmarks = (envelope && envelope.ok && envelope.bookmarks) ? envelope.bookmarks : [];
							bookmarksStatus.textContent = bookmarks.length === 0 ? 'No bookmarks yet. Open a chat, then bookmark its latest message.' : bookmarks.length + ' bookmark(s), message bodies are not stored.';
							bookmarks.forEach(function(bookmark) {
								if (!matchesFilter(bookmark.chat_label || 'Bookmarked message')) return;
								var row = document.createElement('div');
								row.style.cssText = 'display:flex;align-items:center;gap:6px;font-size:11px;';
								var label = document.createElement('span');
								label.className = 'wa-text-primary';
								label.style.cssText = 'flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;';
								var when = bookmark.message_time ? ' · ' + bookmark.message_time : '';
								label.textContent = (bookmark.chat_label || 'Bookmarked message') + when;
								label.title = bookmark.confidence === 'high' ? 'High-confidence identity' : 'Medium-confidence identity (title-based)';
								var mkBtn = function(text, title, handler) {
									var button = document.createElement('button');
									button.className = 'wa-card-btn';
									button.style.cssText = 'padding:2px 6px;border-radius:5px;font-size:10px;cursor:pointer;border-width:1px;border-style:solid;flex-shrink:0;';
									button.textContent = text;
									button.title = title;
									button.onclick = handler;
									return button;
								};
								row.appendChild(label);
								row.appendChild(mkBtn('Open', 'Open this bookmarked message', function() {
									window.waBookmarkOpen(bookmark).then(function(result) {
										if (!result.ok) window.showFloatingToast && window.showFloatingToast('🔖 ' + result.error);
										else if (result.warning) window.showFloatingToast && window.showFloatingToast('🔖 ' + result.warning);
									});
								}));
								row.appendChild(mkBtn('✕', 'Remove bookmark', function() { window.waBookmarkRemove(bookmark.ID).then(renderBookmarks); }));
								bookmarksList.appendChild(row);
							});
						}).catch(function() { bookmarksStatus.textContent = 'Bookmarks unavailable'; });
					};
					var bookmarkAdd = document.getElementById('wa-bookmark-add');
					if (bookmarkAdd) bookmarkAdd.onclick = function() {
						window.waBookmarkLatest().then(function(result) {
							if (result.ok) renderBookmarks();
							else window.showFloatingToast && window.showFloatingToast('🔖 ' + (result.error || 'Could not bookmark this message'));
						});
					};
					var bookmarkClear = document.getElementById('wa-bookmark-clear');
					if (bookmarkClear) bookmarkClear.onclick = function() {
						window.waBookmarkClear().then(renderBookmarks);
					};
					renderBookmarks();

					// Labels, notes, and the shared local filter (M5-04). Notes
					// are rendered with textContent only, never as HTML.
					var labelsList = document.getElementById('wa-labels-list');
					var notesList = document.getElementById('wa-notes-list');
					var filterInput = document.getElementById('wa-prod-filter');
					if (filterInput) filterInput.oninput = function() { window.waProductivityFilter(filterInput.value); };
					window.__waProdRerender = function() { renderPins(); renderBookmarks(); renderNotes(); };
					var matchesFilter = function(text) {
						var filter = window.waProductivityFilterValue();
						return !filter || String(text || '').toLowerCase().indexOf(filter) !== -1;
					};
					var renderLabels = function() {
						if (!labelsList) return;
						window.waLabelList().then(function(envelope) {
							labelsList.innerHTML = '';
							var labels = (envelope && envelope.ok && envelope.labels) ? envelope.labels : [];
							labels.forEach(function(label) {
								var row = document.createElement('div');
								row.style.cssText = 'display:flex;align-items:center;gap:6px;font-size:11px;';
								var dot = document.createElement('span');
								dot.style.cssText = 'width:8px;height:8px;border-radius:50%;flex-shrink:0;background:' + (/^#[0-9a-fA-F]{6}$/.test(label.color) ? label.color : 'transparent') + ';border:1px solid rgba(127,127,127,.35);';
								var name = document.createElement('span');
								name.className = 'wa-text-primary';
								name.style.cssText = 'flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;';
								name.textContent = label.name;
								var mkBtn = function(text, title, handler) {
									var button = document.createElement('button');
									button.className = 'wa-card-btn';
									button.style.cssText = 'padding:2px 6px;border-radius:5px;font-size:10px;cursor:pointer;border-width:1px;border-style:solid;flex-shrink:0;';
									button.textContent = text;
									button.title = title;
									button.onclick = handler;
									return button;
								};
								row.appendChild(dot);
								row.appendChild(name);
								row.appendChild(mkBtn('✕', 'Remove label (detaches it from bookmarks)', function() { window.waLabelRemove(label.ID).then(function() { window.__waProdRerender(); }); }));
								labelsList.appendChild(row);
							});
						}).catch(function() {});
					};
					var labelAdd = document.getElementById('wa-label-add');
					if (labelAdd) labelAdd.onclick = function() {
						var nameInput = document.getElementById('wa-label-name');
						var colorSelect = document.getElementById('wa-label-color');
						window.waLabelAdd(nameInput.value, colorSelect.value).then(function(result) {
							if (result.ok) { nameInput.value = ''; renderLabels(); }
							else window.showFloatingToast && window.showFloatingToast('🏷️ ' + (result.error || 'Could not add label'));
						});
					};
					var noteAdd = document.getElementById('wa-note-add');
					if (noteAdd) noteAdd.onclick = function() {
						var noteInput = document.getElementById('wa-note-new');
						if (!noteInput.value.trim()) return;
						window.waNoteAdd('', '', noteInput.value).then(function(result) {
							if (result.ok) { noteInput.value = ''; renderNotes(); }
							else window.showFloatingToast && window.showFloatingToast('📝 ' + (result.error || 'Could not add note'));
						});
					};
					var renderNotes = function() {
						if (!notesList) return;
						window.waNoteList().then(function(envelope) {
							notesList.innerHTML = '';
							var notes = (envelope && envelope.ok && envelope.notes) ? envelope.notes : [];
							notes.forEach(function(note) {
								if (!matchesFilter(note.text)) return;
								var row = document.createElement('div');
								row.style.cssText = 'display:flex;align-items:flex-start;gap:6px;font-size:11px;';
								var text = document.createElement('span');
								text.className = 'wa-text-primary';
								text.style.cssText = 'flex:1;min-width:0;white-space:pre-wrap;word-break:break-word;';
								text.textContent = note.text;
								var mkBtn = function(text2, title, handler) {
									var button = document.createElement('button');
									button.className = 'wa-card-btn';
									button.style.cssText = 'padding:2px 6px;border-radius:5px;font-size:10px;cursor:pointer;border-width:1px;border-style:solid;flex-shrink:0;';
									button.textContent = text2;
									button.title = title;
									button.onclick = handler;
									return button;
								};
								row.appendChild(text);
								row.appendChild(mkBtn('Edit', 'Edit note', function() {
									if (text.querySelector('textarea')) return;
									var area = document.createElement('textarea');
									area.value = note.text;
									area.maxLength = 8192;
									area.setAttribute('aria-label', 'Edit note');
									area.style.cssText = 'flex:1;min-width:0;padding:2px 4px;font-size:11px;';
									text.textContent = '';
									text.appendChild(area);
									area.focus();
									var commit = function() { window.waNoteUpdate(note.ID, area.value).then(renderNotes); };
									area.onblur = commit;
								}));
								row.appendChild(mkBtn('✕', 'Delete note', function() { window.waNoteRemove(note.ID).then(renderNotes); }));
								notesList.appendChild(row);
							});
						}).catch(function() {});
					};
					renderLabels();
					renderNotes();

					// Appearance card (M6-03/M6-04): reads the stored state and
					// persists through Go-side validation; malformed CSS is
					// rejected and never replaces the last-known-good copy.
					var appearanceState = null;
					var syncAppearanceInputs = function() {
						var set = function(id, prop) { var el = document.getElementById(id); if (el && appearanceState) el.checked = !!appearanceState[prop]; };
						set('wa-appearance-compact', 'compact_mode');
						set('wa-appearance-hide-unread', 'hide_unread_badges');
						set('wa-appearance-hide-archived', 'hide_archived_row');
						var density = document.getElementById('wa-appearance-density');
						if (density && appearanceState) density.value = appearanceState.density || 'comfortable';
						var scale = document.getElementById('wa-appearance-scale');
						if (scale && appearanceState) scale.value = String(appearanceState.scale_percent || 100);
						var cssText = document.getElementById('wa-css-text');
						if (cssText && appearanceState) cssText.value = appearanceState.custom_css || '';
						var cssEnabled = document.getElementById('wa-css-enabled');
						if (cssEnabled && appearanceState) cssEnabled.checked = !!appearanceState.css_enabled;
					};
					if (window.getAppearanceNative) {
						Promise.resolve(window.getAppearanceNative()).then(function(raw) {
							appearanceState = typeof raw === 'string' ? JSON.parse(raw) : raw;
							if (appearanceState && appearanceState.appearance) appearanceState = appearanceState.appearance;
							syncAppearanceInputs();
						}).catch(function() {});
					}
					var appearanceSave = document.getElementById('wa-appearance-save');
					if (appearanceSave) appearanceSave.onclick = function() {
						var next = {};
						var bag = appearanceState || {};
						for (var key in bag) { if (Object.prototype.hasOwnProperty.call(bag, key)) next[key] = bag[key]; }
						next.compact_mode = document.getElementById('wa-appearance-compact').checked;
						next.hide_unread_badges = document.getElementById('wa-appearance-hide-unread').checked;
						next.hide_archived_row = document.getElementById('wa-appearance-hide-archived').checked;
						next.density = document.getElementById('wa-appearance-density').value;
						next.scale_percent = parseInt(document.getElementById('wa-appearance-scale').value, 10) || 100;
						next.custom_css = document.getElementById('wa-css-text').value;
						next.css_enabled = document.getElementById('wa-css-enabled').checked;
						Promise.resolve(window.setAppearanceNative(JSON.stringify(next))).then(function(raw) {
							var result = typeof raw === 'string' ? JSON.parse(raw) : raw;
							if (result && result.ok) {
								appearanceState = result.appearance;
								syncAppearanceInputs();
								window.waAppearanceApply(result);
								window.showFloatingToast && window.showFloatingToast('🎨 Appearance saved');
							} else {
								syncAppearanceInputs();
								window.showFloatingToast && window.showFloatingToast('🎨 ' + ((result && result.error) || 'Invalid appearance change; previous values restored'));
							}
						});
					};
					var appearanceReset = document.getElementById('wa-appearance-reset');
					if (appearanceReset) appearanceReset.onclick = function() {
						var reset = { compact_mode: false, density: 'comfortable', scale_percent: 100, hide_unread_badges: false, hide_archived_row: false, custom_css: (appearanceState && appearanceState.last_known_good_css) || '', css_enabled: false };
						Promise.resolve(window.setAppearanceNative(JSON.stringify(reset))).then(function(raw) {
							var result = typeof raw === 'string' ? JSON.parse(raw) : raw;
							if (result && result.ok) {
								appearanceState = result.appearance;
								syncAppearanceInputs();
								window.waAppearanceApply(result);
								window.showFloatingToast && window.showFloatingToast('🎨 Appearance reset');
							}
						});
					};
					var cssDisable = document.getElementById('wa-css-disable');
					if (cssDisable) cssDisable.onclick = function() { window.waDisableCustomCSS().then(function() { if (window.getAppearanceNative) { Promise.resolve(window.getAppearanceNative()).then(function(raw) { appearanceState = typeof raw === 'string' ? JSON.parse(raw) : raw; if (appearanceState && appearanceState.appearance) appearanceState = appearanceState.appearance; syncAppearanceInputs(); }); } }); };

					var storySave = document.getElementById('wa-story-save');
					if (storySave) storySave.onclick = function() {
						storySave.disabled = true;
						window.waSaveCurrentStory().then(function(result) {
							storySave.disabled = false;
							if (result.ok) window.showFloatingToast && window.showFloatingToast('💾 Saved to downloads');
							else window.showFloatingToast && window.showFloatingToast('📖 ' + (result.error || 'Could not save this story'));
						});
					};

					var trayStatus = document.getElementById('wa-tray-status');
					if (trayStatus && window.getTraySettingsNative) {
						Promise.resolve(window.getTraySettingsNative()).then(function(raw) {
							var tray = typeof raw === 'string' ? JSON.parse(raw) : raw;
							if (!tray || !tray.supported) {
								trayStatus.textContent = 'Tray-hide controls are available on Windows.';
								return;
							}
							trayStatus.textContent = 'Minimize/close to tray keeps the session alive.';
							var toggles = document.getElementById('wa-tray-toggles');
							var mkToggle = function(key, label, checked) {
								var box = document.createElement('label');
								box.className = 'wa-text-muted';
								box.style.cssText = 'font-size:11px;display:flex;align-items:center;gap:4px;cursor:pointer;';
								var input = document.createElement('input');
								input.type = 'checkbox';
								input.checked = !!checked;
								input.setAttribute('aria-label', label);
								input.onchange = function() {
									Promise.resolve(window.setTraySettingsNative(JSON.stringify({
										minimize_to_tray: key === 'minimize' ? input.checked : undefined,
										close_to_tray: key === 'close' ? input.checked : undefined
									}))).then(function(next) {
										var st = typeof next === 'string' ? JSON.parse(next) : next;
										input.checked = !!(st && st[key === 'minimize' ? 'minimize_to_tray' : 'close_to_tray']);
									});
								};
								box.appendChild(input);
								box.appendChild(document.createTextNode(label));
								return box;
							};
							toggles.appendChild(mkToggle('minimize', 'Minimize to tray', tray.minimize_to_tray));
							toggles.appendChild(mkToggle('close', 'Close to tray', tray.close_to_tray));
						}).catch(function() { trayStatus.textContent = 'Tray status unavailable'; });
					} else if (trayStatus) {
						trayStatus.textContent = 'Tray integration not available.';
					}

					var lockStatus = document.getElementById('wa-lock-status');
					var lockSupported = !!window.getLockPolicyNative;
				document.getElementById('wa-lock-manage').disabled = !lockSupported;
				document.getElementById('wa-lock-now').disabled = !window.requestAppLockNative;
				if (!lockSupported) lockStatus.textContent = 'Native app lock is available on Windows and Linux.';
				else Promise.resolve(window.getLockPolicyNative()).then(function(raw) {
						var lock = typeof raw === 'string' ? JSON.parse(raw) : raw;
						lockStatus.textContent = lock.enabled ? 'Enabled · ' + (lock.idle_timeout_seconds ? 'idle ' + lock.idle_timeout_seconds + 's' : 'manual') + (lock.lock_on_minimize ? ' · minimize' : '') + (lock.lock_on_tray ? ' · tray' : '') + (lock.lock_on_startup ? ' · startup' : '') : 'Disabled';
					}).catch(function() { lockStatus.textContent = 'Lock status unavailable'; });
				}
				if (profileSelect) profileSelect.onchange = function() {
					window.selectPrivacyProfile(profileSelect.value).then(refreshPrivacyCard);
				};
				window.addEventListener('wa-privacy-profile-changed', refreshPrivacyCard);
				if (window.refreshPrivacyProfiles) window.refreshPrivacyProfiles().then(refreshPrivacyCard);
				document.getElementById('wa-privacy-copy-custom').onclick = function() {
					if (window.copyCurrentPrivacyProfileToCustom) window.copyCurrentPrivacyProfileToCustom().then(refreshPrivacyCard);
				};
				document.getElementById('wa-privacy-reset').onclick = function() {
					if (window.resetPrivacyProfiles) window.resetPrivacyProfiles().then(refreshPrivacyCard);
				};
				document.getElementById('wa-privacy-reveal-mode').onchange = function() {
					var modifier = document.getElementById('wa-privacy-reveal-modifier').value;
					if (window.setCustomPrivacyReveal) window.setCustomPrivacyReveal(this.value, modifier).then(refreshPrivacyCard);
				};
				document.getElementById('wa-privacy-reveal-modifier').onchange = function() {
					var mode = document.getElementById('wa-privacy-reveal-mode').value;
					if (window.setCustomPrivacyReveal) window.setCustomPrivacyReveal(mode, this.value).then(refreshPrivacyCard);
				};
				document.getElementById('wa-lock-now').onclick = function() {
					if (window.requestNativeAppLock) window.requestNativeAppLock();
				};
				document.getElementById('wa-lock-manage').onclick = function() {
					if (window.manageAppLockNative) Promise.resolve(window.manageAppLockNative()).then(refreshPrivacyCard);
				};
				document.getElementById('wa-action-toggle-pin').onclick = function() {
					if (window.toggleAlwaysOnTop) {
						window.toggleAlwaysOnTop().then(function() { updateBadges(); });
					}
				};
				document.getElementById('wa-action-toggle-mute').onclick = function() {
					if (window.toggleMuteAudio) window.toggleMuteAudio();
					updateBadges();
				};
				document.getElementById('wa-action-toggle-auto').onclick = function() {
					if (window.toggleAutoStart) {
						window.toggleAutoStart().then(function() { updateBadges(); });
					}
				};

				document.getElementById('wa-btn-check-updates-modal').onclick = function() {
					closeSettings();
					if (window.triggerCheckForUpdate) window.triggerCheckForUpdate();
				};
				document.getElementById('wa-btn-reload-modal').onclick = function() {
					if (window.reloadWhatsApp) window.reloadWhatsApp();
				};
				document.getElementById('wa-btn-hardref-modal').onclick = function() {
					if (window.hardRefreshWhatsApp) window.hardRefreshWhatsApp();
				};
				document.getElementById('wa-btn-onboard-modal').onclick = function() {
					closeSettings();
					if (window.showOnboardingModal) window.showOnboardingModal();
				};

				var shortcutsBtn = document.getElementById('wa-btn-show-shortcuts');
				var shortcutsList = document.getElementById('wa-shortcuts-list');
				if (shortcutsBtn && shortcutsList) {
					var modifier = isMac ? 'Cmd' : 'Ctrl';
					shortcutsList.innerHTML =
						'<div><strong class="wa-text-primary">' + modifier + '+,</strong> &mdash; Settings &amp; Controls</div>' +
						'<div><strong class="wa-text-primary">' + modifier + '+Shift+D</strong> &mdash; Open downloads folder</div>' +
						'<div><strong class="wa-text-primary">' + modifier + '+Shift+U</strong> &mdash; Check for updates</div>' +
						'<div><strong class="wa-text-primary">' + modifier + '+Shift+P / T / M / S</strong> &mdash; Privacy / on top / mute / startup</div>' +
						'<div><strong class="wa-text-primary">Esc</strong> &mdash; Close this window</div>';
					shortcutsBtn.onclick = function() {
						var open = shortcutsList.style.display !== 'none';
						shortcutsList.style.display = open ? 'none' : 'block';
						shortcutsBtn.textContent = open ? 'View keyboard shortcuts' : 'Hide keyboard shortcuts';
					};
				}

				var diagnosticsBtn = document.getElementById('wa-btn-run-diagnostics');
				var diagnosticsResult = document.getElementById('wa-diagnostics-result');
				if (diagnosticsBtn && diagnosticsResult) {
					diagnosticsBtn.onclick = function() {
						var checks = [];
					var requiredBindings = ['getDownloadDirNative', 'openDownloadDirNative', 'checkForUpdateNative', 'checkFileExistsNative'];
						var missing = requiredBindings.filter(function(name) { return typeof window[name] !== 'function'; });
						checks.push(missing.length ? 'Native bridge: unavailable (' + missing.join(', ') + ')' : 'Native bridge: ready');
						checks.push(navigator.onLine === false ? 'Network: offline (chat may not refresh)' : 'Network: available');
						try {
							var key = 'wa-desk-diagnostic-probe';
							localStorage.setItem(key, '1');
							localStorage.removeItem(key);
							checks.push('Local settings storage: ready');
						} catch (e) { checks.push('Local settings storage: unavailable (preferences will not persist)'); }
						var healthy = !missing.length && navigator.onLine !== false;
						diagnosticsResult.style.display = 'block';
						diagnosticsResult.style.background = healthy ? 'rgba(0,168,132,.10)' : 'rgba(234,0,56,.10)';
						diagnosticsResult.innerHTML = '<strong class="wa-text-primary">' + (healthy ? 'Quick check complete' : 'Attention needed') + '</strong><br>' + checks.map(function(line) { return '• ' + line; }).join('<br>');
					};
				}

				// Populate current download dir
				var pathLabel = document.getElementById('wa-folder-path');
				if (window.getDownloadDirNative) {
					window.getDownloadDirNative().then(function(dir) {
						if (pathLabel) pathLabel.textContent = dir;
					});
				}

				// Change folder action
				document.getElementById('wa-btn-change-folder').onclick = function() {
					if (window.chooseDownloadDirNative) {
						window.chooseDownloadDirNative().then(function(newDir) {
							if (newDir && pathLabel) {
								pathLabel.textContent = newDir;
								showFloatingToast('📁 Downloads folder updated!');
							}
						});
					}
				};

				// Open folder action
				document.getElementById('wa-btn-open-folder').onclick = function() {
					if (window.openDownloadDirNative) {
						window.openDownloadDirNative();
						showFloatingToast('📁 Opening folder in file manager...');
					}
				};

				// Reset folder action
				document.getElementById('wa-btn-reset-folder').onclick = function() {
					if (window.resetDownloadDirNative) {
						window.resetDownloadDirNative().then(function(defDir) {
							if (pathLabel) pathLabel.textContent = defDir;
							showFloatingToast('📁 Downloads folder reset to default.');
						});
					}
				};

				var organizeBox = document.getElementById('wa-organize-month');
				if (organizeBox) {
					if (window.getOrganizeByMonthNative) {
						window.getOrganizeByMonthNative().then(function(on) {
							organizeBox.checked = !!on;
						}).catch(function() {});
					}
					organizeBox.onchange = function() {
						if (!window.setOrganizeByMonthNative) return;
						window.setOrganizeByMonthNative(organizeBox.checked).then(function(applied) {
							showFloatingToast(applied ?
								'🗂️ Downloads will be organized into monthly folders.' :
								'🗂️ Downloads save directly to the folder again.');
						}).catch(function() {});
					};
				}
			};

		});

		} catch (waInitError) {
			// A module above failed (an engine API difference, denied
			// storage, ...). Record it and keep going: everything below
			// must still be installed.
			waNoteRecoverable('init-body', waInitError);
		}

		// Escape must close the active WhatsApp chat without invoking macOS's
		// default fullscreen exit behavior. When no chat is active, keep the
		// window fullscreen as well while allowing WhatsApp to handle its own UI.
		// Native/app-owned overlays keep their existing Escape handlers and are
		// deliberately excluded here.
		waRunModule('escape-chat', function() {
			if (__WA_GOOS !== 'darwin') return;
			function nativeOverlayOpen() {
				return !!document.querySelector('#wa-settings-overlay, #wa-doc-modal-overlay, #wa-onboarding-overlay, #wa-recovery-overlay') ||
					!!window.waDOM.resolveFirst('mediaViewer', document).node;
			}
			function activeChatHeader() {
				return window.waDOM.resolveFirst('conversationHeader', document).node;
			}
			function closeChatFromEscape() {
				if (nativeOverlayOpen()) return false;
				var header = activeChatHeader();
				if (!header) return false;
				var back = header.querySelector([
					'button[data-testid="back"]', '[data-testid="back"]',
					'[data-icon="back"]', 'button[aria-label*="Back" i]',
					'[role="button"][aria-label*="Back" i]',
					'button[aria-label*="Kembali" i]',
					'[role="button"][aria-label*="Kembali" i]',
					'button[title*="Back" i]'
				].join(','));
				if (back) {
					var control = back.closest && back.closest('button, [role="button"]');
					(control || back).click();
					return true;
				}
				return false;
			}
			window.addEventListener('keydown', function(e) {
				if (e.key !== 'Escape' || !e.isTrusted || nativeOverlayOpen()) return;
				e.preventDefault();
				if (closeChatFromEscape()) e.stopImmediatePropagation();
			}, true);
		});

		// --- Core shortcuts (self-contained) ------------------------------
		// Registered outside the modules above on purpose. The Settings button
		// and Cmd/Ctrl+, used to disappear together on Windows because a single
		// exception in an earlier module aborted the rest of the injected
		// script. The controls the user needs to RECOVER from such a state must
		// not depend on the modules that can break.
		waRunModule('core-shortcuts', function() {
			// Cmd/Ctrl+, opens Settings; Cmd/Ctrl+Shift+D opens the downloads
			// folder. Both are also reachable from the Control Center itself.
			function isSettingsChord(e) {
				return (e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey &&
					(e.key === ',' || e.key === '<' || e.code === 'Comma');
			}
			window.addEventListener('keydown', function(e) {
				if (isSettingsChord(e)) {
					e.preventDefault();
					e.stopPropagation();
					// Never let a broken Control Center swallow the chord: the
					// shortcut is a recovery path, so it must not throw.
					try {
						if (typeof window.showSettingsModal === 'function') {
							window.showSettingsModal();
						} else if (typeof window.openRecoveryPanel === 'function') {
							window.openRecoveryPanel();
						}
					} catch (err) {
						waNoteRecoverable('shortcut-settings', err);
						if (typeof window.openRecoveryPanel === 'function') {
							window.openRecoveryPanel();
						}
					}
				} else if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'd' || e.key === 'D')) {
					e.preventDefault();
					e.stopPropagation();
					try {
						if (typeof window.openDownloadDirNative === 'function') {
							window.openDownloadDirNative();
							if (typeof window.showFloatingToast === 'function') {
								window.showFloatingToast('📁 Opening downloads folder...');
							}
						}
					} catch (err) {
						waNoteRecoverable('shortcut-downloads', err);
					}
				}
			}, true);
		});
		// --- Emergency Settings entry point -------------------------------
		// Guarantees a way into Settings even when the Control Center module
		// above failed to install. Deliberately depends on nothing but the DOM:
		// if the real button exists it defers to it, otherwise it mounts a
		// minimal launcher, and if even the modal is missing the launcher opens
		// a recovery panel with the recorded failures.
		waRunModule('emergency-settings', function() {
			function realEntryPointPresent() {
				return !!document.getElementById('wa-toolbar-settings-btn') ||
					!!document.getElementById('wa-settings-fallback-btn');
			}
			function showRecoveryPanel() {
				var existing = document.getElementById('wa-recovery-overlay');
				if (existing && existing.parentNode) { existing.parentNode.removeChild(existing); return; }
				var fails = (typeof window.__waRecoverable === 'function') ? window.__waRecoverable() : [];
				var overlay = document.createElement('div');
				overlay.id = 'wa-recovery-overlay';
				overlay.style.cssText = 'position:fixed;inset:0;background:rgba(8,15,19,.72);z-index:2147483647;display:flex;align-items:center;justify-content:center;padding:20px;box-sizing:border-box;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif;';
				var card = document.createElement('div');
				card.style.cssText = 'width:440px;max-width:94vw;max-height:80vh;overflow:auto;background:#111b21;color:#e9edef;border:1px solid rgba(134,150,160,.35);border-radius:10px;padding:18px 20px;box-shadow:0 18px 48px rgba(0,0,0,.4);font-size:12.5px;line-height:1.6;';
				var rows = fails.length
					? fails.map(function(f) { return '<li>' + String(f).replace(/[<>&]/g, '') + '</li>'; }).join('')
					: '<li>No failures recorded.</li>';
				card.innerHTML =
					'<strong style="font-size:14px;">WhatsApp Desk — recovery</strong>' +
					'<p style="opacity:.8;margin:8px 0 10px;">The Settings panel did not load. Recorded problems:</p>' +
					'<ul style="margin:0 0 14px;padding-left:18px;opacity:.9;">' + rows + '</ul>' +
					'<button id="wa-recovery-reload" style="background:#00a884;color:#111b21;border:none;padding:8px 14px;border-radius:6px;font-weight:600;cursor:pointer;">Reload WhatsApp Web</button>';
				overlay.appendChild(card);
				document.body.appendChild(overlay);
				var closeXForFocus = document.getElementById('wa-settings-close-x');
				if (closeXForFocus) closeXForFocus.focus();
				var btn = document.getElementById('wa-recovery-reload');
				if (btn) {
					btn.onclick = function() {
						if (typeof window.reloadWhatsApp === 'function') { window.reloadWhatsApp(); }
						else { window.location.reload(); }
					};
				}
			}
			function openSettings() {
				if (typeof window.showSettingsModal === 'function') {
					try { window.showSettingsModal(); return; } catch (e) { waNoteRecoverable('showSettingsModal', e); }
				}
				showRecoveryPanel();
			}
			window.openRecoveryPanel = showRecoveryPanel;
			// The last-resort launcher must never sit next to a working control:
			// it is a lifeline for the case where the other entry points failed,
			// not an extra gear. So it only mounts while no *visible* entry point
			// exists, and unmounts again as soon as one appears.
			function visibleEntryPointPresent() {
				var ids = ['wa-toolbar-settings-btn', 'wa-settings-fallback-btn'];
				for (var i = 0; i < ids.length; i++) {
					var el = document.getElementById(ids[i]);
					if (!el || el.style.display === 'none') continue;
					var r = el.getBoundingClientRect();
					if (r.width > 0 && r.height > 0) return true;
				}
				return false;
			}
			function mount() {
				var existing = document.getElementById('wa-emergency-settings-btn');
				if (visibleEntryPointPresent()) {
					if (existing && existing.parentNode) existing.parentNode.removeChild(existing);
					return;
				}
				if (existing) return;
				if (!document.body) return;
				var btn = document.createElement('button');
				btn.id = 'wa-emergency-settings-btn';
				btn.type = 'button';
				btn.setAttribute('aria-label', 'Open Settings and Controls');
				// This module has its own scope; compute the modifier locally
				// instead of referencing the outer isMac, which is undefined here
				// and previously threw, preventing the last-resort button mounting.
				var emgIsMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0;
				btn.title = 'Settings & Controls (' + (emgIsMac ? 'Cmd' : 'Ctrl') + ' + ,)';
				btn.innerHTML = '<svg width="21" height="21" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>';
				btn.style.cssText = 'position:fixed;left:14px;bottom:14px;z-index:2147483646;width:38px;height:38px;padding:0;display:inline-flex;align-items:center;justify-content:center;border:1px solid rgba(134,150,160,.45);border-radius:50%;background:#111b21;color:#aebac1;cursor:pointer;box-shadow:0 4px 12px rgba(0,0,0,.3);';
				btn.onclick = function(e) { e.preventDefault(); e.stopPropagation(); openSettings(); };
				document.body.appendChild(btn);
			}
			// The rail fallback announces every mount/hide so this launcher can
			// re-evaluate: it must disappear the moment a real control appears.
			window.__waRecheckEmergencySettings = mount;
			mount();
			document.addEventListener('DOMContentLoaded', mount);
			window.addEventListener('load', mount);
			// The Control Center mounts asynchronously after WhatsApp's own boot.
			setTimeout(mount, 1200);
			setTimeout(mount, 4000);
		});

	` + "\n" + getOnboardingScript()
	script = strings.Replace(script, "// __WA_DOM_ADAPTER__", domAdapterSource, 1)
	// Single source of truth: every UI version string flows from appVersion
	// (overridable at link time via -ldflags "-X main.appVersion=...").
	return strings.ReplaceAll(script, "__WA_APP_VERSION__", appVersion)
}

type WindowState struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
	Maximized bool    `json:"maximized,omitempty"`
	// Screens maps a stable display identifier (macOS NSScreenNumber) to the
	// frame the window had on that monitor. Only macOS populates it; other
	// platforms round-trip it unchanged.
	Screens map[string]WindowState `json:"screens,omitempty"`
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			writeCrashReport("main", r)
		}
	}()
	runApp()
}
