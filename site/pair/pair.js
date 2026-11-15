const q = new URLSearchParams(location.hash.slice(1));
const ticket = {
  a: q.get('a') || '',
  c: q.get('c') || '',
  f: q.get('f') || '',
  h: q.get('h') || '',
};
const name = (q.get('n') || '').slice(0, 40);
const friend = q.get('friend') === '1';
if (name) ticket.n = name;
if (friend) ticket.friend = '1';
const problem = document.getElementById('problem');
const ok = /^[0-9A-Za-z.:[\]-]{3,100}:[0-9]{1,5}$/.test(ticket.a) &&
  /^[0-9A-Za-z-]{4,32}$/.test(ticket.c) &&
  /^[0-9A-Za-z_-]{43}$/.test(ticket.f) &&
  ticket.h.length <= 63;

if (/iPhone|iPad/.test(navigator.userAgent)) {
  problem.textContent = friend
    ? 'This link is for an Android phone. Ask for a link for an iPhone.'
    : 'This page is for Android phones. For an iPhone, run cloche pair -ios on your computer.';
  problem.hidden = false;
  document.getElementById('steps').hidden = true;
} else if (!ok) {
  problem.textContent = 'This link is missing its pairing code. Run cloche pair -android on your computer and scan the code it shows.';
  problem.hidden = false;
} else {
  if (name || ticket.h) {
    document.getElementById('title').textContent = 'Pair this phone with ' + (name || ticket.h);
  }
  if (friend) {
    document.getElementById('tailscale').textContent = 'Install it and sign in with your own account. Then accept the invite you got to this computer.';
    document.getElementById('again').textContent = 'If the link has run out, ask for a new one.';
    document.getElementById('trust').hidden = false;
  }
  const open = document.getElementById('open');
  open.href = 'cloche://pair?' + new URLSearchParams(ticket).toString();
  open.removeAttribute('aria-disabled');
  document.getElementById('code').textContent = 'Code ' + ticket.c;
}
