'use strict';

const $ = (s, el = document) => el.querySelector(s);
const main = $('#main');
const standalone = navigator.standalone === true || matchMedia('(display-mode: standalone)').matches;
let me = null;
let poll = null;

function h(tag, attrs = {}, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v == null || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
    else el.setAttribute(k, v === true ? '' : v);
  }
  for (const kid of kids.flat(Infinity)) {
    if (kid == null || kid === false) continue;
    el.append(kid instanceof Node ? kid : document.createTextNode(String(kid)));
  }
  return el;
}

function show(...nodes) {
  main.replaceChildren(...nodes.flat().filter(Boolean));
}

async function api(path, opts = {}) {
  const r = await fetch(path, { cache: 'no-store', ...opts, headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) } });
  let body = null;
  try { body = await r.json(); } catch {}
  if (!r.ok) {
    const e = new Error((body && body.error) || r.statusText || 'request failed');
    e.status = r.status;
    throw e;
  }
  return body;
}

let toastTimer;
function toast(msg, bad = false) {
  const t = $('#toast');
  t.textContent = msg;
  t.className = 'on' + (bad ? ' red' : '');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.className = bad ? 'red' : ''), bad ? 6000 : 3000);
}

function ago(iso) {
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 45) return 'just now';
  if (s < 3600) return Math.round(s / 60) + 'm ago';
  if (s < 86400) return Math.round(s / 3600) + 'h ago';
  if (s < 7 * 86400) return Math.round(s / 86400) + 'd ago';
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

const day = (iso) => new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
const mb = (n) => (n / 1048576).toFixed(1) + ' MB';
const hostName = () => (me && me.host) || stored('host') || 'your computer';

function stored(k, v) {
  try {
    if (v === undefined) return localStorage.getItem(k);
    localStorage.setItem(k, v);
  } catch {}
  return null;
}

function go(path, replace = false) {
  history[replace ? 'replaceState' : 'pushState'](null, '', path);
  route();
}

document.addEventListener('click', (e) => {
  const a = e.target.closest('a[data-go]');
  if (!a) return;
  e.preventDefault();
  go(a.getAttribute('data-go'));
});
addEventListener('popstate', route);

function icon(b, big = false) {
  const el = h('div', { class: 'icon' + (big ? ' big' : '') });
  if (b.icon) el.append(h('img', { src: `/m/${b.id}/icon.png`, alt: '' }));
  else el.textContent = (b.name || '?').trim().charAt(0).toUpperCase();
  return el;
}

async function load() {
  try {
    me = await api('/api/me');
    stored('host', me.host);
  } catch (e) {
    offline(e);
    return;
  }
  $('#host').textContent = me.paired ? `${me.name} · ${me.host}` : me.host;
  if (me.paired && 'setAppBadge' in navigator) {
    const n = me.builds.filter((b) => b.status === 'new').length;
    (n ? navigator.setAppBadge(n) : navigator.clearAppBadge()).catch(() => {});
  }
  const q = new URLSearchParams(location.search);
  if (q.has('udid')) {
    history.replaceState(null, '', location.pathname);
    if (me.udid) toast('Got your iPhone’s ID');
  }
  route();
  resubscribe();
}

function offline(e) {
  const host = hostName();
  const tailnet = !e.status || e.status === 503;
  show(
    h('div', { class: 'sheet' },
      h('h1', {}, tailnet ? `Can’t reach ${host}` : 'Something went wrong'),
      tailnet
        ? h('p', { class: 'dim' }, `Cloche reaches ${host} over Tailscale. Check Tailscale is on, and connected.`)
        : h('p', { class: 'dim' }, e.message),
      h('div', { class: 'row' }, h('button', { onclick: load }, 'Try again')),
    ),
  );
}

function route() {
  clearInterval(poll);
  poll = null;
  if (!me) return;
  if (!me.paired) return pairPage();
  const m = location.pathname.match(/^\/b\/([a-z0-9]+)$/);
  if (m) return buildPage(m[1]);
  if (location.pathname !== '/') history.replaceState(null, '', '/');
  homePage();
}

let linkCode = '';
let linkTried = false;

function pairPage() {
  if (location.hash.length > 1) {
    linkCode = decodeURIComponent(location.hash.slice(1));
    linkTried = false;
    history.replaceState(null, '', '/pair');
  }
  const code = linkCode;
  const host = me.host;
  if (!me.pairing) {
    show(h('div', { class: 'sheet' },
      h('h1', {}, `This iPhone isn’t paired with ${host}`),
      h('p', { class: 'dim' }, code
        ? 'This pairing link has been used, or it’s run out. Ask for a new one.'
        : `Ask whoever runs ${host} for a pairing link.`),
    ));
    return;
  }
  const input = h('input', { value: code, placeholder: 'XXXX-XXXX-XXXX', autocomplete: 'one-time-code', autocapitalize: 'characters', spellcheck: 'false', inputmode: 'text', 'aria-label': 'Pairing code' });
  const err = h('p', { class: 'red' });
  const btn = h('button', { class: 'wide', type: 'submit' }, 'Pair this iPhone');
  const form = h('form', { onsubmit: (e) => { e.preventDefault(); pair(input.value, btn, err); } }, input, h('div', { class: 'row' }, btn), err);
  show(h('div', { class: 'sheet' },
    h('h1', {}, `Pair with ${host}`),
    h('p', { class: 'dim' }, `${host} sends builds of the apps it makes to this iPhone. You install each with a tap.`),
    form,
  ));
  if (code && !linkTried) {
    linkTried = true;
    pair(code, btn, err);
  }
}

async function pair(code, btn, err) {
  btn.disabled = true;
  btn.textContent = 'Pairing…';
  err.textContent = '';
  try {
    const r = await api('/api/pair', { method: 'POST', body: JSON.stringify({ code }) });
    history.replaceState(null, '', '/');
    toast(`Paired as ${r.name}`);
    await load();
  } catch (e) {
    err.textContent = e.message;
    btn.disabled = false;
    btn.textContent = 'Pair this iPhone';
  }
}

function homePage() {
  const host = me.host;
  const notifyOn = me.notify && 'Notification' in window && Notification.permission === 'granted';
  const steps = [];
  const sent = stored('profile-sent');

  steps.push(step(steps.length + 1, me.udid, 'Send your iPhone’s ID',
    me.udid
      ? [`${host} has it${me.model ? ` (${me.model})` : ''}.`]
      : [
          `Apps are signed for a list of iPhones. This sends ${host} your iPhone’s ID to go on the list.`,
          h('div', { class: 'row' }, h('a', { class: 'button', href: '/udid.mobileconfig', onclick: () => { stored('profile-sent', '1'); setTimeout(homePage, 400); } }, 'Get the profile')),
          sent && h('p', {}, 'Then open Settings, tap Profile Downloaded near the top, and tap Install. iOS sends the ID and opens this page again.'),
        ]));
  if (!standalone) {
    steps.push(step(steps.length + 1, false, 'Add Cloche to your Home Screen', [
      'In Safari, tap Share, then Add to Home Screen. Open Cloche from your Home Screen for the last step.',
    ]));
  }
  steps.push(step(steps.length + 1, standalone ? notifyOn : me.notify, 'Turn on notifications',
    standalone
      ? notifyOn
        ? ['A notification comes with each build.']
        : ['A notification comes with each build.', h('div', { class: 'row' }, h('button', { onclick: (e) => turnOnNotifications(e.target) }, 'Turn on notifications'))]
      : [me.notify ? 'They’re on in the Home Screen app.' : 'From Cloche on your Home Screen.']));

  const setupDone = me.udid && (standalone ? notifyOn : me.notify);
  const builds = me.builds.map((b) =>
    h('li', {}, h('a', { class: 'build', href: `/b/${b.id}`, 'data-go': `/b/${b.id}` },
      icon(b),
      h('div', {},
        h('div', { class: 'name' }, `${b.name} ${b.version}`),
        h('div', { class: 'meta' }, [b.note || b.bundle_id, ago(b.pushed_at)].join(' · ')),
      ),
      tag(b.status),
    )));

  show(
    !setupDone && [h('h2', {}, 'Set up'), h('div', { class: 'sheet' }, h('ol', { class: 'steps' }, steps))],
    h('h2', {}, 'Builds'),
    builds.length
      ? h('ul', { class: 'builds' }, builds)
      : h('div', { class: 'sheet empty' },
          h('img', { src: '/icon-180.png', alt: '' }),
          h('p', {}, 'No builds yet'),
          h('p', { class: 'dim' }, `Builds from ${host} show up here${me.notify ? ', with a notification' : ''}.`)),
    h('footer', {},
      h('p', {}, `Paired with ${host} as ${me.name}`),
      h('button', { class: 'quiet', onclick: unpair }, 'Unpair this iPhone')),
  );
}

function step(n, done, title, body) {
  return h('li', { class: done ? 'done' : '' },
    done ? tick() : h('span', { class: 'n', 'aria-hidden': 'true' }, String(n)),
    h('span', { class: 't' }, title),
    h('div', { class: 'b' }, body.map((x) => (typeof x === 'string' ? h('p', {}, x) : x))));
}

function tick() {
  const el = h('span', { class: 'n', 'aria-label': 'done' });
  el.innerHTML = '<svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true"><path d="M2 6.5 4.8 9 10 3" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="square"/></svg>';
  return el;
}

function tag(status) {
  switch (status) {
    case 'new': return h('span', { class: 'tag new' }, 'New');
    case 'asked': return h('span', { class: 'tag busy' }, 'Asked');
    case 'downloading': return h('span', { class: 'tag busy' }, 'Loading');
    case 'downloaded': return h('span', { class: 'tag' }, 'Sent');
  }
  return h('span');
}

async function unpair() {
  if (!confirm(`Unpair this iPhone from ${me.host}? Builds stop coming until it pairs again.`)) return;
  try {
    await api('/api/unpair', { method: 'POST', body: '{}' });
    const reg = await navigator.serviceWorker?.getRegistration();
    const sub = await reg?.pushManager?.getSubscription();
    await sub?.unsubscribe();
  } catch (e) {
    toast(e.message, true);
  }
  load();
}

function key(b64) {
  const s = atob(b64.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (b64.length % 4)) % 4));
  return Uint8Array.from(s, (c) => c.charCodeAt(0));
}

function sameKey(a, b) {
  if (!a) return false;
  const x = new Uint8Array(a);
  return x.length === b.length && x.every((v, i) => v === b[i]);
}

async function subscription(fresh = false) {
  const reg = await navigator.serviceWorker.ready;
  const want = key(me.vapid);
  let sub = await reg.pushManager.getSubscription();
  if (sub && (fresh || !sameKey(sub.options && sub.options.applicationServerKey, want))) {
    await sub.unsubscribe();
    sub = null;
  }
  return sub || reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: want });
}

async function subscribe() {
  let r = await api('/api/notify', { method: 'POST', body: JSON.stringify(await subscription()) });
  if (!r.sent && r.gone) r = await api('/api/notify', { method: 'POST', body: JSON.stringify(await subscription(true)) });
  return r;
}

async function turnOnNotifications(btn) {
  if (!('Notification' in window) || !('serviceWorker' in navigator)) {
    toast('This needs iOS 16.4 or later, from the Home Screen', true);
    return;
  }
  btn.disabled = true;
  const perm = await Notification.requestPermission();
  if (perm !== 'granted') {
    btn.disabled = false;
    toast('Notifications are off for Cloche: Settings, Notifications, Cloche', true);
    return;
  }
  try {
    const r = await subscribe();
    if (r.sent) toast('Notifications are on');
    else toast('The first notification didn’t go through: ' + r.error, true);
  } catch (e) {
    toast(e.message, true);
  }
  load();
}

let resubscribed = false;
async function resubscribe() {
  if (resubscribed || !standalone || !me.paired || me.notify) return;
  if (!('Notification' in window) || Notification.permission !== 'granted') return;
  resubscribed = true;
  try {
    const r = await subscribe();
    if (r.sent) {
      me.notify = true;
      if (location.pathname === '/') homePage();
    }
  } catch {}
}

async function buildPage(id) {
  let b;
  try {
    b = await api(`/api/builds/${id}`);
  } catch (e) {
    if (e.status === 404) {
      toast('That build isn’t here any more', true);
      go('/', true);
    } else offline(e);
    return;
  }
  const host = me.host;
  const status = h('div', { class: 'status' });
  const blocked = b.covers === false;
  const install = b.sideload
    ? h('a', { class: 'button wide', href: b.download, download: '', onclick: () => watch(id, status) }, 'Download the .ipa')
    : h('a', {
        class: 'button wide' + (blocked ? ' off' : ''),
        href: blocked ? null : b.install,
        onclick: () => {
          setStatus(status, 'asked', b);
          watch(id, status);
        },
      }, 'Install');

  const rows = [['App', b.bundle_id]];
  if (b.source && b.source.commit) {
    let from = `${b.source.repo}@${b.source.commit}`;
    if (b.source.dirty) from += ' with changes';
    if (b.source.branch && b.source.branch !== 'HEAD') from += ` on ${b.source.branch}`;
    rows.push(['From', from]);
  }
  rows.push(['Sent', `${ago(b.pushed_at)} by ${host}`]);
  rows.push(['Size', mb(b.size)]);
  if (b.kind) rows.push(['Signed', [b.kind === 'ad hoc' ? 'Ad hoc' : b.kind[0].toUpperCase() + b.kind.slice(1), b.team].filter(Boolean).join(', ')]);
  else if (b.sideload) rows.push(['Signed', 'Unsigned']);
  if (b.expires) rows.push(['Works until', day(b.expires)]);
  if (b.min_os) rows.push(['Needs', `iOS ${b.min_os}`]);

  show(
    h('p', {}, h('a', { href: '/', 'data-go': '/' }, '‹ All builds')),
    h('div', { class: 'sheet' },
      h('div', { class: 'title' }, icon(b, true), h('div', {}, h('h1', {}, b.name), h('div', { class: 'dim' }, b.version))),
      b.note && h('p', { class: 'note' }, b.note),
      h('dl', {}, rows.map(([k, v]) => [h('dt', {}, k), h('dd', {}, v)])),
      blocked && h('p', { class: 'callout red' }, `This iPhone isn’t in the build’s profile, and iOS won’t install it. Ask ${host} for a build signed for it.`),
      b.sideload && h('p', { class: 'callout' }, 'It’s unsigned, for SideStore or AltStore to sign with your Apple ID. Download it, then open it from Files in SideStore or AltStore.'),
      !b.sideload && b.covers == null && b.kind !== 'enterprise' && h('p', { class: 'callout' }, 'Cloche can check a build is signed for this iPhone once it has your iPhone’s ID. See Set up on the first page.'),
      b.kind === 'development' && h('p', { class: 'callout' }, 'A development build: it opens with Developer Mode on, in Settings, Privacy & Security, Developer Mode.'),
      h('div', { class: 'row' }, install),
      status,
    ),
    h('div', { class: 'row' }, h('button', { class: 'quiet', onclick: () => clear(id) }, 'Clear from the list')),
  );
  setStatus(status, b.status, b);
  if (b.status === 'asked' || b.status === 'downloading') watch(id, status);
}

function setStatus(el, s, b) {
  const host = me.host;
  switch (s) {
    case 'asked':
      el.replaceChildren('iOS asks whether to install it. Tap Install there, then go to your Home Screen to watch it load.');
      break;
    case 'downloading':
      el.replaceChildren(h('span', { class: 'dot' }), `Downloading from ${host}…`);
      break;
    case 'downloaded':
      el.replaceChildren(h('span', { class: 'live' }, b.sideload
        ? `Downloaded ${ago(b.status_at)}. It’s in Files, in Downloads: open it in SideStore or AltStore.`
        : `Downloaded ${ago(b.status_at)}. iOS installs it from there: look for ${b.name} on your Home Screen.`));
      break;
    default:
      el.replaceChildren();
  }
}

function watch(id, el) {
  clearInterval(poll);
  const until = Date.now() + 10 * 60 * 1000;
  poll = setInterval(async () => {
    if (document.hidden) return;
    if (Date.now() > until) return clearInterval(poll);
    try {
      const b = await api(`/api/builds/${id}`);
      setStatus(el, b.status, b);
      if (b.status === 'downloaded') clearInterval(poll);
    } catch {}
  }, 2000);
}

async function clear(id) {
  try {
    await api(`/api/builds/${id}/clear`, { method: 'POST', body: '{}' });
  } catch (e) {
    toast(e.message, true);
  }
  await load();
  go('/', true);
}

if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js').catch(() => {});
  navigator.serviceWorker.addEventListener('message', (e) => {
    if (e.data && e.data.type === 'go') {
      history.pushState(null, '', e.data.url);
      load();
    }
  });
}
document.addEventListener('visibilitychange', () => {
  if (!document.hidden && me) load();
});
$('#host').textContent = stored('host') || '';
load();
