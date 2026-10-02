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

function element(name, selectorMatches = [], attrs = {}, children = {}) {
  return {
    name,
    nodeType: 1,
    parentElement: null,
    matches(selector) { return selectorMatches.includes(selector); },
    getAttribute(attr) { return attrs[attr] !== undefined ? attrs[attr] : null; },
    setAttribute(attr, value) { attrs[attr] = value; },
    querySelectorAll(selector) { return children[selector] || []; },
    textContent: '',
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
  assert.equal(ambiguousAPI.version, 2, 'adapter version tracks the identity contract');
  console.log('PASS: preferred/fallback/missing/ambiguous resolution, DOM ordering, privacy surface selectors, and closest-element behavior');
}

function runIdentity() {
  const chatRowSelectors = ['[data-testid="cell-frame-container"]'];
  const titleSelectors = ['[data-testid="cell-frame-title"]'];

  // High confidence: a data-id attribute anywhere in the row yields an
  // opaque hashed key; the raw WhatsApp attribute must never leak.
  const rowWithID = element('row with data-id', chatRowSelectors, { 'data-id': '6281234567890@c.us' });
  const withID = install({}).api.chatRowIdentity(rowWithID);
  assert.equal(withID.status, 'found');
  assert.equal(withID.kind, 'data-id');
  assert.equal(withID.confidence, 'high');
  assert.match(withID.key, /^chat:[0-9a-f]{16}$/);
  assert.ok(!withID.key.includes('@'), 'the key must be opaque, not a raw JID');

  // Deterministic and distinct per chat.
  const sameAgain = install({}).api.chatRowIdentity(rowWithID);
  assert.equal(sameAgain.key, withID.key);
  const rowOther = element('other row', chatRowSelectors, { 'data-id': '6289999999999@c.us' });
  assert.notEqual(install({}).api.chatRowIdentity(rowOther).key, withID.key);

  // Medium confidence: title fallback is disclosed as such and changes on rename.
  const titleEl = element('title', titleSelectors, { title: 'Budi' });
  const rowWithTitle = element('row with title', chatRowSelectors, {}, { [titleSelectors[0]]: [titleEl] });
  const withTitle = install({}).api.chatRowIdentity(rowWithTitle);
  assert.equal(withTitle.status, 'found');
  assert.equal(withTitle.kind, 'title-fallback');
  assert.equal(withTitle.confidence, 'medium');
  assert.match(withTitle.key, /^chat:[0-9a-f]{16}$/);
  titleEl.setAttribute('title', 'Budi Kantor');
  const renamed = install({}).api.chatRowIdentity(rowWithTitle);
  assert.notEqual(renamed.key, withTitle.key, 'title-fallback keys must track the visible title');

  // Missing: no data-id and no title must produce no key at all.
  const blankRow = element('blank row', chatRowSelectors);
  const missing = install({}).api.chatRowIdentity(blankRow);
  assert.equal(missing.status, 'missing');
  assert.equal(missing.key, '');

  // chatRowByKey resolves the row for a stored key and fails closed on
  // unknown or ambiguous matches.
  const rowsAPI = install({ [chatRowSelectors[0]]: [rowWithID, rowOther] }).api;
  const foundRow = rowsAPI.chatRowByKey(withID.key);
  assert.equal(foundRow.status, 'found');
  assert.equal(foundRow.key, withID.key);
  assert.equal(rowsAPI.chatRowByKey('chat:deadbeefdeadbeef').status, 'missing');

  // Ambiguity: two rows matching one key (hash collision fixture) fail closed.
  const collisionRow = element('collision row', chatRowSelectors, { 'data-id': '6281234567890@c.us' });
  const collisionAPI = install({ [chatRowSelectors[0]]: [rowWithID, collisionRow] }).api;
  assert.equal(collisionAPI.chatRowByKey(withID.key).status, 'ambiguous');

  // Message identity: data-id on the message wrapper is required.
  const messageWrapper = element('message wrapper', [], { 'data-id': 'true_6281234567890@c.us_3EB0FDFE' });
  const inner = element('message text');
  inner.parentElement = messageWrapper;
  const msgID = install({}).api.messageIdentity(inner);
  assert.equal(msgID.status, 'found');
  assert.equal(msgID.kind, 'data-id');
  assert.equal(msgID.confidence, 'high');
  assert.match(msgID.key, /^msg:[0-9a-f]{16}$/);
  const noID = element('bare message');
  noID.parentElement = null;
  assert.equal(install({}).api.messageIdentity(noID).status, 'missing');

  // Active conversation identity: title fallback from the open chat header.
  const mainRoot = element('conversation root', ['#main'], {}, {
    [titleSelectors[0]]: [element('header title', [], { title: 'Budi' })],
  });
  const headerTitleSelectors = ['#main header span[title]'];
  const headerTitle = element('header title span', headerTitleSelectors, { title: 'Budi' });
  const mainRoot2 = element('conversation root 2', ['#main'], {}, {
    [headerTitleSelectors[0]]: [headerTitle],
  });
  const activeAPI = install({ '#main': [mainRoot2] }).api;
  const active = activeAPI.activeChatIdentity();
  assert.equal(active.status, 'found');
  assert.equal(active.kind, 'title-fallback');
  assert.equal(active.confidence, 'medium');
  assert.equal(active.label, 'Budi');
  assert.match(active.key, /^chat:[0-9a-f]{16}$/);
  const noChatAPI = install({}).api;
  assert.equal(noChatAPI.activeChatIdentity().status, 'missing');
  assert.equal(noChatAPI.activeChatIdentity().key, '');

  // Latest message identity: the last identifiable message in DOM order
  // wins; containers without identity are skipped, and absence fails closed.
  const oldMsg = element('old message', [], { 'data-id': 'true_chat_111' });
  const newMsg = element('new message', [], { 'data-id': 'true_chat_222' });
  const unidentifiable = element('system bubble');
  const msgWrapperJoined = install({}).api.selectors('messageWrapper').join(', ');
  const mainForMessages = element('conversation', ['#main'], {}, {
    [msgWrapperJoined]: [oldMsg, newMsg, unidentifiable],
  });
  const latestAPI = install({ '#main': [mainForMessages] }).api;
  const latest = latestAPI.latestMessageIdentity();
  assert.equal(latest.status, 'found');
  const expectedNew = latestAPI.messageIdentity(newMsg);
  assert.equal(latest.key, expectedNew.key, 'the last identifiable message must win');
  const emptyMain = element('empty conversation', ['#main']);
  const emptyAPI = install({ '#main': [emptyMain] }).api;
  assert.equal(emptyAPI.latestMessageIdentity().status, 'missing');
  const noMainAPI = install({}).api;
  assert.equal(noMainAPI.latestMessageIdentity().status, 'missing');

  // messageNodeByKey: found for a loaded message, ambiguous on collision,
  // missing when the message is not in the visible conversation.
  const foundMsg = latestAPI.messageNodeByKey(latestAPI.messageIdentity(newMsg).key);
  assert.equal(foundMsg.status, 'found');
  const collisionMsg = element('duplicate message', [], { 'data-id': 'true_chat_222' });
  const msgCollisionMain = element('conversation', ['#main'], {}, {
    [msgWrapperJoined]: [newMsg, collisionMsg],
  });
  const msgCollisionAPI = install({ '#main': [msgCollisionMain] }).api;
  assert.equal(msgCollisionAPI.messageNodeByKey(latestAPI.messageIdentity(newMsg).key).status, 'ambiguous');
  assert.equal(latestAPI.messageNodeByKey('msg:0000000000000000').status, 'missing');

  console.log('PASS: chat/message identity keys are opaque, confidence-tagged, deterministic, and fail closed on ambiguity');
}

run();
runIdentity();
