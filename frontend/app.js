// ---------------------------------------------------------------
// Surge Queue — frontend logic
// Talks to two real services (no mock data, no invented endpoints):
//   Queue Engine (Go)   -> QUEUE_BASE
//   Backend (Spring Boot) -> BACKEND_BASE
//
// Update these two lines when you deploy — everything else in this
// file works unchanged against localhost or a real domain.
// ---------------------------------------------------------------
const QUEUE_BASE = 'http://localhost:8080';
const BACKEND_BASE = 'http://localhost:8081';

const POLL_INTERVAL_MS = 1500;
const HEARTBEAT_EVERY_N_POLLS = 4; // heartbeat roughly every ~6s

// ---- DOM refs ----
const panels = {
  landing: document.getElementById('panel-landing'),
  waiting: document.getElementById('panel-waiting'),
  admitted: document.getElementById('panel-admitted'),
  done: document.getElementById('panel-done'),
};
const tracker = document.getElementById('tracker');

const inputName = document.getElementById('input-name');
const inputEmail = document.getElementById('input-email');
const btnJoin = document.getElementById('btn-join');
const landingError = document.getElementById('landing-error');

const statPosition = document.getElementById('stat-position');
const statAhead = document.getElementById('stat-ahead');
const statEta = document.getElementById('stat-eta');
const statState = document.getElementById('stat-state');

const regName = document.getElementById('reg-name');
const regEmail = document.getElementById('reg-email');
const regDetails = document.getElementById('reg-details');
const btnRegister = document.getElementById('btn-register');
const admittedError = document.getElementById('admitted-error');

const ticketId = document.getElementById('ticket-id');

// ---- State ----
let userId = sessionStorage.getItem('surgeQueueUserId') || null;
let admitToken = null;
let pollTimer = null;
let pollCount = 0;

// ---- Helpers ----
function showPanel(name) {
  Object.entries(panels).forEach(([key, el]) => {
    el.hidden = key !== name;
  });
  updateTracker(name);
}

function updateTracker(panelName) {
  const stepFor = { landing: 'waiting', waiting: 'waiting', admitted: 'admitted', done: 'done' };
  const activeState = stepFor[panelName] || 'waiting';
  const order = ['waiting', 'admitted', 'done'];
  const activeIndex = order.indexOf(activeState);

  [...tracker.children].forEach((li) => {
    const stepIndex = order.indexOf(li.dataset.state);
    li.classList.remove('is-active', 'is-done');
    if (stepIndex < activeIndex) li.classList.add('is-done');
    else if (stepIndex === activeIndex) li.classList.add('is-active');
  });
}

function formatEta(seconds) {
  if (seconds === undefined || seconds === null) return '\u2014';
  if (seconds <= 0) return 'any moment';
  if (seconds < 60) return `~${seconds}s`;
  const mins = Math.round(seconds / 60);
  return `~${mins} min`;
}

async function postJSON(url, body, headers) {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...(headers || {}) },
    body: JSON.stringify(body || {}),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(data.message || data.error || `Request failed (${res.status})`);
    err.status = res.status;
    err.data = data;
    throw err;
  }
  return data;
}

async function getJSON(url) {
  const res = await fetch(url);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(data.message || data.error || `Request failed (${res.status})`);
    err.status = res.status;
    throw err;
  }
  return data;
}

// ---- Flow: join queue ----
btnJoin.addEventListener('click', async () => {
  landingError.hidden = true;

  if (!inputName.value.trim() || !inputEmail.value.trim()) {
    landingError.textContent = 'Enter your name and email to continue.';
    landingError.hidden = false;
    return;
  }

  btnJoin.disabled = true;
  btnJoin.textContent = 'Joining\u2026';

  try {
    const result = await postJSON(`${QUEUE_BASE}/api/queue/enter`, {});
    userId = result.user_id;
    sessionStorage.setItem('surgeQueueUserId', userId);

    // carry the name/email forward to prefill registration once admitted
    sessionStorage.setItem('surgeQueueName', inputName.value.trim());
    sessionStorage.setItem('surgeQueueEmail', inputEmail.value.trim());

    applyStatus(result.status);
    showPanel('waiting');
    startPolling();
  } catch (err) {
    landingError.textContent = 'Could not reach the queue. Check your connection and try again.';
    landingError.hidden = false;
  } finally {
    btnJoin.disabled = false;
    btnJoin.textContent = 'Join the queue';
  }
});

function applyStatus(status) {
  if (!status) return;
  statPosition.textContent = status.position ?? '\u2014';
  statAhead.textContent = Math.max(0, (status.position ?? 1) - 1);
  statEta.textContent = formatEta(status.eta_seconds);
  statState.textContent = status.state || 'waiting';

  if (status.state === 'admitted' && status.admit_token) {
    admitToken = status.admit_token;
    stopPolling();
    enterAdmittedPanel();
  }
}

function startPolling() {
  pollCount = 0;
  pollTimer = setInterval(async () => {
    if (!userId) return;
    pollCount += 1;
    try {
      const status = await getJSON(`${QUEUE_BASE}/api/queue/status/${userId}`);
      applyStatus(status);

      if (pollCount % HEARTBEAT_EVERY_N_POLLS === 0 && status.state === 'waiting') {
        postJSON(`${QUEUE_BASE}/api/queue/heartbeat/${userId}`, {}).catch(() => {});
      }
    } catch (err) {
      // transient network hiccups shouldn't stop the poll loop
    }
  }, POLL_INTERVAL_MS);
}

function stopPolling() {
  if (pollTimer) clearInterval(pollTimer);
  pollTimer = null;
}

function enterAdmittedPanel() {
  regName.value = sessionStorage.getItem('surgeQueueName') || '';
  regEmail.value = sessionStorage.getItem('surgeQueueEmail') || '';
  showPanel('admitted');
}

// ---- Flow: registration ----
btnRegister.addEventListener('click', async () => {
  admittedError.hidden = true;

  if (!regName.value.trim() || !regEmail.value.trim()) {
    admittedError.textContent = 'Enter your name and email to finish.';
    admittedError.hidden = false;
    return;
  }
  if (!admitToken) {
    admittedError.textContent = 'Your queue token expired. Refresh and rejoin the queue.';
    admittedError.hidden = false;
    return;
  }

  btnRegister.disabled = true;
  btnRegister.textContent = 'Submitting\u2026';

  try {
    const result = await postJSON(
      `${BACKEND_BASE}/api/register`,
      {
        name: regName.value.trim(),
        email: regEmail.value.trim(),
        details: regDetails.value.trim() || undefined,
      },
      { Authorization: `Bearer ${admitToken}` }
    );

    ticketId.textContent = result.registration_id || '\u2014';
    sessionStorage.removeItem('surgeQueueUserId');
    showPanel('done');
  } catch (err) {
    if (err.status === 401) {
      admittedError.textContent = 'Your queue token was rejected. Refresh and rejoin the queue.';
    } else {
      admittedError.textContent = 'Registration didn\u2019t go through. Try again in a moment.';
    }
    admittedError.hidden = false;
  } finally {
    btnRegister.disabled = false;
    btnRegister.textContent = 'Complete registration';
  }
});

// ---- Resume mid-queue on reload ----
(async function init() {
  showPanel('landing');
  if (userId) {
    try {
      const status = await getJSON(`${QUEUE_BASE}/api/queue/status/${userId}`);
      showPanel('waiting');
      applyStatus(status);
      if (status.state === 'waiting') startPolling();
    } catch {
      sessionStorage.removeItem('surgeQueueUserId');
      userId = null;
    }
  }
})();
