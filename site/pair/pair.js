const q = new URLSearchParams(location.hash.slice(1));
const ticket = {
  a: q.get('a') || '',
  c: q.get('c') || '',
  f: q.get('f') || '',
  h: q.get('h') || '',
};
const problem = document.getElementById('problem');
const ok = /^[0-9A-Za-z.:[\]-]{3,100}:[0-9]{1,5}$/.test(ticket.a) &&
  /^[0-9A-Za-z-]{4,32}$/.test(ticket.c) &&
  /^[0-9A-Za-z_-]{43}$/.test(ticket.f) &&
  ticket.h.length <= 63;

if (/iPhone|iPad/.test(navigator.userAgent)) {
  problem.textContent = 'This page is for Android phones. For an iPhone, run cloche pair -ios on your computer.';
  problem.hidden = false;
  document.getElementById('steps').hidden = true;
} else if (!ok) {
  problem.textContent = 'This link is missing its pairing code. Run cloche pair on your computer and scan the code it shows.';
  problem.hidden = false;
} else {
  if (ticket.h) {
    document.getElementById('title').textContent = 'Pair this phone with ' + ticket.h;
  }
  const open = document.getElementById('open');
  open.href = 'cloche://pair?' + new URLSearchParams(ticket).toString();
  open.removeAttribute('aria-disabled');
  document.getElementById('code').textContent = 'Code ' + ticket.c;
}
