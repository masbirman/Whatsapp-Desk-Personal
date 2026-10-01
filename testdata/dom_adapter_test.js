'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

const source = fs.readFileSync(process.argv[2], 'utf8');

function install(queryMap) {
  const calls = [];
  const document = {
    querySelectorAll(selector) {
      calls.push(selector);
      return queryMap[selector] || [];
    },
  };
  const window = {};
  vm.runInNewContext(source, { window, document, Object, Array });
  return { api: window.waDOM, calls };
}

function element(name, selectorMatches = []) {
  return {
    name,
    nodeType: 1,
    parentElement: null,
    matches(selector) { return selectorMatches.includes(selector); },
  };
}

function run() {
  const preferred = element('preferred chat list');
  const laterFallback = element('later fallback');
  const first = install({
    '#side': [preferred],
    '[data-testid="chat-list"]': [laterFallback],
  });
  const preferredResult = first.api.resolveOne('chatListRoot');
  assert.equal(preferredResult.status, 'found');
  assert.equal(preferredResult.node, preferred);
  assert.deepEqual(first.calls, ['#side'], 'the adapter must stop at the first working selector');

  const fallbackNode = element('chat list fallback');
  const fallback = install({ '[data-testid="chat-list"]': [fallbackNode] });
  const fallbackResult = fallback.api.resolveOne('chatListRoot');
  assert.equal(fallbackResult.status, 'found');
  assert.equal(fallbackResult.node, fallbackNode);
  assert.deepEqual(fallback.calls, ['#side', '#pane-side', '[data-testid="chat-list"]']);

  const missing = install({}).api.resolveOne('mediaViewer');
  assert.equal(missing.status, 'missing');
  assert.equal(missing.node, null);

  const ambiguousNodes = [element('viewer 1'), element('viewer 2')];
  const ambiguousAPI = install({ '[data-testid="media-viewer"]': ambiguousNodes }).api;
  const ambiguous = ambiguousAPI.resolveOne('mediaViewer');
  assert.equal(ambiguous.status, 'ambiguous');
  assert.equal(ambiguous.node, null, 'identity-sensitive resolveOne must fail closed');
  assert.equal(ambiguous.nodes.length, 2);

  const row = element('chat row', ['[role="row"]']);
  const target = element('row child');
  target.parentElement = row;
  const closest = install({}).api.closest(target, 'chatRow');
  assert.equal(closest.status, 'found');
  assert.equal(closest.node, row);

  const mediaAPI = install({}).api;
  const closeSelector = mediaAPI.selectors('mediaViewerCloseControl')[0];
  const externalClose = element('close control outside viewer');
  const scopedViewer = { querySelectorAll() { return []; } };
  const fallbackDocument = {
    querySelectorAll(selector) { return selector === closeSelector ? [externalClose] : []; },
  };
  const closeCandidates = mediaAPI.resolveCandidates('mediaViewerCloseControl', scopedViewer, fallbackDocument);
  assert.equal(closeCandidates[0].node, externalClose, 'fallback root should preserve legacy close-control lookup');

  const downloadSelectors = mediaAPI.selectors('mediaViewerDownloadControl');
  const firstInDOM = element('first control in DOM', [downloadSelectors[2]]);
  const secondInDOM = element('second control in DOM', [downloadSelectors[0]]);
  const ordered = [firstInDOM, secondInDOM];
  const orderedAPI = install({ [downloadSelectors.join(', ')]: ordered }).api;
  const orderedCandidates = orderedAPI.resolveCandidatesInDOMOrder('mediaViewerDownloadControl');
  assert.equal(orderedCandidates[0].node, firstInDOM, 'DOM-order resolution should retain querySelectorAll ordering');
  assert.equal(orderedCandidates[1].node, secondInDOM);

  const viewerSelectors = mediaAPI.selectors('mediaViewer');
  const firstViewerInDOM = element('first viewer', [viewerSelectors[1]]);
  const secondViewerInDOM = element('second viewer', [viewerSelectors[0]]);
  const viewerAPI = install({ [viewerSelectors.join(', ')]: [firstViewerInDOM, secondViewerInDOM] }).api;
  const firstViewer = viewerAPI.resolveFirstInDOMOrder('mediaViewer');
  assert.equal(firstViewer.status, 'ambiguous');
  assert.equal(firstViewer.node, firstViewerInDOM, 'legacy first viewer selection should follow DOM order');

  const privacyKeys = [
    'privacyChatNames', 'privacyGroupIndicator', 'privacyChatPreviews', 'privacyTimestamps',
    'privacyUnreadCounts', 'privacyAvatars', 'privacyMessageText', 'privacyImages',
    'privacyVideos', 'privacyStickers', 'privacyQuotedContent', 'privacyVoiceNoteDetails',
    'privacyHeaderNames', 'privacyHeaderAvatars', 'privacyHeaderSubtitles', 'privacyViewerMedia',
  ];
  privacyKeys.forEach((key) => {
    const selectors = mediaAPI.selectors(key);
    assert.ok(selectors.length > 0, `privacy selector registry is missing ${key}`);
    assert.ok(selectors.every((selector) => selector.length > 0), `${key} has an empty selector`);

    const preferredNode = element(`${key} preferred`);
    const preferredAPI = install({ [selectors[0]]: [preferredNode] }).api;
    const preferredCandidates = preferredAPI.resolveCandidates(key);
    assert.equal(preferredCandidates.length, 1, `${key} should resolve its preferred selector`);
    assert.equal(preferredCandidates[0].node, preferredNode);

    if (selectors.length > 1) {
      const fallbackNode = element(`${key} fallback`);
      const fallbackAPI = install({ [selectors[selectors.length - 1]]: [fallbackNode] }).api;
      const fallbackCandidates = fallbackAPI.resolveCandidates(key);
      assert.equal(fallbackCandidates.length, 1, `${key} should resolve its last fallback selector`);
      assert.equal(fallbackCandidates[0].node, fallbackNode);
    }
  });
  const previewSelectors = mediaAPI.selectors('privacyChatPreviews');
  const previewNode = element('message preview', [previewSelectors[0]]);
  const timestampNode = element('timestamp', [mediaAPI.selectors('privacyTimestamps')[0]]);
  const privacyFixture = install({
    [previewSelectors[0]]: [previewNode],
    [mediaAPI.selectors('privacyTimestamps')[0]]: [timestampNode],
  }).api;
  assert.equal(privacyFixture.resolveCandidates('privacyChatPreviews').length, 1);
  assert.equal(privacyFixture.resolveCandidates('privacyTimestamps').length, 1);

  assert.equal(ambiguousAPI.selectors('unknownFeature').length, 0);
  assert.equal(ambiguousAPI.version, 1);
  console.log('PASS: preferred/fallback/missing/ambiguous resolution, DOM ordering, privacy surface selectors, and closest-element behavior');
}

run();
