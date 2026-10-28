const ICONS = {
  compass: '<circle cx="12" cy="12" r="9"/><path d="m15.4 8.6-2 4.8-4.8 2 2-4.8 4.8-2Z"/>',
  dashboard: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  plane: '<path d="M17.8 19.2 16 11l3.5-3.5a2.1 2.1 0 0 0-3-3L13 8 4.8 6.2a1 1 0 0 0-.9 1.7l5.6 4.1-3.2 3.2-2.3-.5a1 1 0 0 0-.9 1.7l3.1 2.4 2.4 3.1a1 1 0 0 0 1.7-.9l-.5-2.3 3.2-3.2 4.1 5.6a1 1 0 0 0 1.7-.9Z"/>',
  bed: '<path d="M3 20V8"/><path d="M3 17h18v3"/><path d="M21 17v-4a3 3 0 0 0-3-3h-6V6a1 1 0 0 0-1-1H3"/><path d="M7 10h1"/>',
  car: '<path d="m5 11 1.5-4h11L19 11"/><path d="M4 11h16a1 1 0 0 1 1 1v4a1 1 0 0 1-1 1h-1a2 2 0 0 1-4 0H9a2 2 0 0 1-4 0H4a1 1 0 0 1-1-1v-4a1 1 0 0 1 1-1Z"/><path d="M7 15h.01M17 15h.01"/>',
  map: '<path d="m3 6 6-3 6 3 6-3v15l-6 3-6-3-6 3V6Z"/><path d="M9 3v15M15 6v15"/>',
  calendar: '<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M16 3v4M8 3v4M3 11h18"/>',
  mail: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="m3 7 9 6 9-6"/>',
  users: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75"/>',
  sparkles: '<path d="m12 3 1.6 4.4L18 9l-4.4 1.6L12 15l-1.6-4.4L6 9l4.4-1.6L12 3Z"/><path d="m19 15 .8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8L19 15Z"/><path d="m5 14 .6 1.4L7 16l-1.4.6L5 18l-.6-1.4L3 16l1.4-.6L5 14Z"/>',
  chart: '<path d="M3 3v18h18"/><path d="m7 15 3-4 3 2 5-7"/>',
  bell: '<path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9"/><path d="M10 21h4"/>',
  logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5M21 12H9"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  arrowLeft: '<path d="m12 19-7-7 7-7M19 12H5"/>',
  arrowRight: '<path d="M5 12h14M12 5l7 7-7 7"/>',
  share: '<circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><path d="m8.6 10.6 6.8-3.9M8.6 13.4l6.8 3.9"/>',
  sync: '<path d="M21 12a9 9 0 0 1-15.5 6.2L3 16"/><path d="M3 12a9 9 0 0 1 15.5-6.2L21 8"/><path d="M3 21v-5h5M21 3v5h-5"/>',
  trash: '<path d="M3 6h18M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6M14 11v6"/>',
  edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  x: '<path d="m6 6 12 12M18 6 6 18"/>',
  link: '<path d="M10 13a5 5 0 0 0 7.5.5l2-2a5 5 0 0 0-7-7l-1.1 1"/><path d="M14 11a5 5 0 0 0-7.5-.5l-2 2a5 5 0 0 0 7 7l1.1-1"/>',
  globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a15 15 0 0 1 0 18a15 15 0 0 1 0-18Z"/>',
  settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-1.8 1.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-2.6V20a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1-1.8-1.8.1-.1A1.7 1.7 0 0 0 8 15a1.7 1.7 0 0 0-1.6-1H6v-2.6h.2a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9l-.1-.1 1.8-1.8.1.1a1.7 1.7 0 0 0 1.9.3 1.7 1.7 0 0 0 1-1.6V5h2.6v.2a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1 1.8 1.8-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2V14h-.2a1.7 1.7 0 0 0-1.6 1Z"/>',
  refresh: '<path d="M20 11a8 8 0 1 0-2.3 5.7"/><path d="M20 4v7h-7"/>',
  star: '<path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2-5.6-2.9-5.6 2.9 1.1-6.2L3 9.6l6.2-.9L12 3Z"/>',
  send: '<path d="m22 2-7 20-4-9-9-4 20-7Z"/><path d="M22 2 11 13"/>',
  external: '<path d="M15 3h6v6M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
  alert: '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z"/><path d="M12 9v4M12 17h.01"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  archive: '<rect x="3" y="4" width="18" height="5" rx="1"/><path d="M5 9v10a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9M10 13h4"/>',
  location: '<path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z"/><circle cx="12" cy="10" r="3"/>',
  copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
  history: '<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5M12 7v5l4 2"/>',
  tag: '<path d="M20 12 12 20l-8-8V4h8l8 8Z"/><path d="M8 8h.01"/>',
  shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z"/><path d="m9 12 2 2 4-4"/>',
  network: '<circle cx="12" cy="5" r="2"/><circle cx="5" cy="19" r="2"/><circle cx="19" cy="19" r="2"/><path d="M12 7v4M12 11 6.5 17.2M12 11l5.5 6.2"/>',
};

export function icon(name, size = 18, className = '') {
  const body = ICONS[name] || ICONS.compass;
  return `<svg class="${className}" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${body}</svg>`;
}

export function esc(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

const dateFmt = new Intl.DateTimeFormat('es-AR', { day: '2-digit', month: 'short', year: 'numeric', timeZone: 'UTC' });
const dateTimeFmt = new Intl.DateTimeFormat('es-AR', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });
const shortFmt = new Intl.DateTimeFormat('es-AR', { day: '2-digit', month: 'short', timeZone: 'UTC' });

function asDate(value) {
  if (!value) return null;
  const d = value instanceof Date ? value : new Date(value);
  return Number.isNaN(d.getTime()) ? null : d;
}

export function formatDate(value) {
  const d = asDate(value);
  return d ? dateFmt.format(d) : 'Sin fecha';
}

export function formatDateTime(value) {
  const d = asDate(value);
  return d ? dateTimeFmt.format(d) : 'Sin fecha';
}

export function formatRange(start, end) {
  const a = asDate(start);
  const b = asDate(end);
  if (!a && !b) return 'Fechas por definir';
  if (!a) return `Hasta ${formatDate(b)}`;
  if (!b) return `Desde ${formatDate(a)}`;
  return `${shortFmt.format(a)} — ${formatDate(b)}`;
}

export function formatYear(value) {
  const d = asDate(value);
  return d ? d.getUTCFullYear() : '';
}

export function formatMoney(cents, currency = 'ARS') {
  const n = Number(cents || 0) / 100;
  try {
    return new Intl.NumberFormat('es-AR', { style: 'currency', currency, maximumFractionDigits: 0 }).format(n);
  } catch {
    return `$${n.toLocaleString('es-AR')}`;
  }
}

export function relativeTime(value) {
  const d = asDate(value);
  if (!d) return '';
  const diff = Date.now() - d.getTime();
  const mins = Math.round(diff / 60000);
  if (Math.abs(mins) < 1) return 'ahora';
  if (mins > 0) {
    if (mins < 60) return `hace ${mins} min`;
    const hours = Math.round(mins / 60);
    if (hours < 24) return `hace ${hours} h`;
    const days = Math.round(hours / 24);
    if (days < 30) return `hace ${days} d`;
    return formatDate(d);
  }
  const future = Math.round(-mins / 1440);
  return future === 0 ? 'hoy' : `en ${future} d`;
}

export function daysUntil(value) {
  const d = asDate(value);
  if (!d) return null;
  return Math.ceil((d.getTime() - Date.now()) / 86400000);
}

export function initials(name = '') {
  return name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((p) => p[0]?.toUpperCase() || '')
    .join('') || 'TT';
}

export function toLocalInput(value) {
  const d = asDate(value);
  if (!d) return '';
  const pad = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function fromLocalInput(value) {
  if (!value) return null;
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}

const TYPE_LABELS = { flight: 'Vuelo', hotel: 'Alojamiento', car: 'Auto', other: 'Otro' };
const TYPE_ICONS = { flight: 'plane', hotel: 'bed', car: 'car', other: 'tag' };

export function typeLabel(type) {
  return TYPE_LABELS[type] || 'Reserva';
}

export function typeIcon(type) {
  return TYPE_ICONS[type] || 'tag';
}

const STATUS_LABELS = {
  active: 'Activo',
  archived: 'Archivado',
  confirmed: 'Confirmada',
  canceled: 'Cancelada',
  pending: 'Pendiente',
  processing: 'Procesando',
  processed: 'Procesado',
  ignored: 'Ignorado',
  error: 'Error',
  open: 'Abierta',
  closed: 'Cerrada',
};

const STATUS_CLASSES = {
  active: 'green',
  archived: 'gray',
  confirmed: 'green',
  canceled: 'red',
  pending: 'amber',
  processing: 'blue',
  processed: 'green',
  ignored: 'gray',
  error: 'red',
  open: 'blue',
  closed: 'gray',
};

export function statusBadge(status) {
  const label = STATUS_LABELS[status] || status || '—';
  return `<span class="badge badge-${STATUS_CLASSES[status] || 'gray'}">${esc(label)}</span>`;
}

const SOURCE_LABELS = { manual: 'Manual', email: 'Email', provider: 'Proveedor' };

export function sourceLabel(source) {
  return SOURCE_LABELS[source] || source || '—';
}

export function networkName(network) {
  const names = {
    x: 'X (Twitter)', twitter: 'X (Twitter)', facebook: 'Facebook', linkedin: 'LinkedIn',
    instagram: 'Instagram', threads: 'Threads',
  };
  return names[network] || network;
}

export function bars(items, emptyText = 'Sin datos para el período') {
  if (!items || items.length === 0) {
    return `<div class="muted small" style="padding:8px 0">${esc(emptyText)}</div>`;
  }
  const max = Math.max(...items.map((i) => Number(i.count) || 0), 1);
  return `<div class="bars">${items.map((item) => {
    const pct = Math.max(3, Math.round(((Number(item.count) || 0) / max) * 100));
    return `<div class="bar-row">
      <span class="bar-label" title="${esc(item.key)}">${esc(item.key || '—')}</span>
      <span class="bar-track"><span class="bar-fill" style="width:${pct}%"></span></span>
      <span class="bar-value">${esc(item.count)}</span>
    </div>`;
  }).join('')}</div>`;
}

export function emptyState(iconName, title, text, action = '') {
  return `<div class="empty">
    <div class="empty-icon">${icon(iconName, 24)}</div>
    <strong>${esc(title)}</strong>
    <p>${esc(text)}</p>
    ${action}
  </div>`;
}

export function toast(message, type = 'success') {
  const root = document.getElementById('toasts');
  if (!root) return;
  const el = document.createElement('div');
  el.className = `toast ${type}`;
  const iconName = type === 'error' ? 'alert' : type === 'info' ? 'info' : 'check';
  el.innerHTML = `${icon(iconName, 17)}<span>${esc(message)}</span>`;
  root.appendChild(el);
  setTimeout(() => {
    el.classList.add('leaving');
    setTimeout(() => el.remove(), 220);
  }, 4200);
}

let closeCurrentModal = null;

export function openModal({ title, subtitle = '', body = '', footer = '', wide = false, onMount }) {
  if (closeCurrentModal) closeCurrentModal();
  const root = document.getElementById('modal-root');
  root.innerHTML = `<div class="modal-overlay" data-modal-overlay>
    <div class="modal ${wide ? 'wide' : ''}" role="dialog" aria-modal="true" aria-label="${esc(title)}">
      <div class="modal-head">
        <div>
          <div class="modal-title">${esc(title)}</div>
          ${subtitle ? `<div class="modal-sub">${esc(subtitle)}</div>` : ''}
        </div>
        <button class="modal-close" type="button" data-modal-close aria-label="Cerrar">${icon('x', 18)}</button>
      </div>
      <div class="modal-body">${body}</div>
      ${footer ? `<div class="modal-foot">${footer}</div>` : ''}
    </div>
  </div>`;
  const overlay = root.querySelector('[data-modal-overlay]');
  const previousOverflow = document.body.style.overflow;
  document.body.style.overflow = 'hidden';
  let closed = false;
  const close = () => {
    if (closed) return;
    closed = true;
    document.body.style.overflow = previousOverflow;
    document.removeEventListener('keydown', onKey);
    root.innerHTML = '';
    closeCurrentModal = null;
  };
  const onKey = (ev) => {
    if (ev.key === 'Escape') close();
  };
  overlay.addEventListener('mousedown', (ev) => {
    if (ev.target === overlay) close();
  });
  root.querySelectorAll('[data-modal-close]').forEach((btn) => btn.addEventListener('click', close));
  document.addEventListener('keydown', onKey);
  closeCurrentModal = close;
  const firstInput = root.querySelector('input:not([type=hidden]), select, textarea');
  if (firstInput) setTimeout(() => firstInput.focus(), 40);
  if (onMount) onMount(root, close);
  return close;
}

export function confirmDialog({ title = 'Confirmar', message, confirmLabel = 'Confirmar', danger = false }) {
  return new Promise((resolve) => {
    let settled = false;
    const finish = (value) => {
      if (settled) return;
      settled = true;
      close();
      resolve(value);
    };
    const close = openModal({
      title,
      body: `<p class="small" style="line-height:1.6">${esc(message)}</p>`,
      footer: `<button class="btn btn-outline" type="button" data-cancel>Cancelar</button>
        <button class="btn ${danger ? 'btn-danger' : 'btn-primary'}" type="button" data-confirm>${esc(confirmLabel)}</button>`,
      onMount(root) {
        root.querySelector('[data-cancel]').addEventListener('click', () => finish(false));
        root.querySelector('[data-confirm]').addEventListener('click', () => finish(true));
      },
    });
  });
}

export function setBusy(form, busy, label) {
  const button = form.querySelector('button[type=submit]');
  if (!button) return;
  if (busy) {
    button.dataset.label = button.innerHTML;
    button.disabled = true;
    button.innerHTML = `<span class="spinner" style="width:16px;height:16px;border-color:rgba(255,255,255,.4);border-top-color:#fff"></span> ${esc(label || 'Guardando...')}`;
  } else {
    button.disabled = false;
    if (button.dataset.label) button.innerHTML = button.dataset.label;
  }
}

export function parseDetails(value) {
  const text = String(value || '').trim();
  if (!text) return {};
  const parsed = JSON.parse(text);
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('Los detalles deben ser un objeto JSON');
  }
  return parsed;
}
