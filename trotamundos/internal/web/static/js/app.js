import { api, session, logout } from './api.js';
import {
  icon, esc, formatDate, formatDateTime, formatRange, formatMoney, relativeTime, daysUntil,
  initials, toLocalInput, fromLocalInput, typeLabel, typeIcon, statusBadge, sourceLabel,
  networkName, bars, emptyState, toast, openModal, confirmDialog,
} from './ui.js';

const app = document.getElementById('app');
const state = {
  user: session.user,
  authMode: 'login',
  route: 'dashboard',
  trips: { active: [], archived: [], tab: 'active', query: '' },
  currentTrip: null,
  currentShares: [],
  mail: { accounts: [], events: [], whitelist: [], filters: [] },
  agencies: [],
  favorite: null,
  recommendations: [],
  requests: [],
  sharedWithMe: [],
  sharedSent: [],
  notifications: [],
  networks: {},
  mockKey: localStorage.getItem('trotamundos.mockKey') || 'dev-mock-key',
  analyticsRange: defaultRange(),
};

const PAGE_META = {
  dashboard: ['Resumen', 'Tu próximo viaje empieza acá'],
  trips: ['Viajes', 'Itinerarios agrupados automáticamente'],
  shared: ['Compartidos', 'Viajes que compartís y viajes que recibís'],
  mail: ['Correo', 'Reservas detectadas en tus emails'],
  agency: ['Agencia', 'Recomendaciones y consultas de tu agencia favorita'],
  analytics: ['Analítica', 'Métricas anonimizadas del ecosistema'],
  notifications: ['Notificaciones', 'Novedades de tus reservas y viajes'],
};

initialize();

async function initialize() {
  if (!session.access && !session.refresh) {
    showAuth();
    return;
  }
  try {
    const me = await api('/auth/me');
    state.user = me;
    session.setUser(me);
    startApp();
  } catch {
    session.clear();
    showAuth();
  }
}

function startApp() {
  if (!location.hash) history.replaceState(null, '', '#/dashboard');
  renderShell();
  loadNotifications().catch(() => {});
  renderRoute();
}

function showAuth() {
  state.user = null;
  const mode = state.authMode;
  app.innerHTML = `
    <div class="auth-page">
      <section class="auth-art">
        <div class="auth-brand">
          <span class="mark">${icon('compass', 24)}</span>
          <span>Trotamundos</span>
        </div>
        <div class="auth-copy">
          <div class="auth-kicker">Todos tus viajes, en un solo lugar</div>
          <h1>Menos caos.<br>Más <em>kilómetros</em>.</h1>
          <p>Centralizá reservas, ordená tus itinerarios y descubrí la próxima escapada con tu agencia de confianza.</p>
        </div>
        <div class="auth-points">
          <span class="auth-point">${icon('mail', 15)} Ingesta de correos</span>
          <span class="auth-point">${icon('map', 15)} Itinerarios automáticos</span>
          <span class="auth-point">${icon('users', 15)} Viajes compartidos</span>
        </div>
      </section>
      <section class="auth-panel">
        <div class="auth-card">
          <h2>${mode === 'login' ? 'Bienvenido de nuevo' : 'Creá tu cuenta'}</h2>
          <p>${mode === 'login' ? 'Ingresá para ver tus viajes y reservas.' : 'Empezá a ordenar tus viajes en minutos.'}</p>
          <div class="auth-tabs">
            <button type="button" class="${mode === 'login' ? 'active' : ''}" data-action="auth-mode" data-mode="login">Ingresar</button>
            <button type="button" class="${mode === 'register' ? 'active' : ''}" data-action="auth-mode" data-mode="register">Registrarme</button>
          </div>
          <form class="auth-form" data-form="auth" data-mode="${mode}" novalidate>
            ${mode === 'register' ? `<label class="field"><span>Nombre completo</span><input name="full_name" autocomplete="name" placeholder="Ana Pérez" required></label>` : ''}
            <label class="field"><span>Email</span><input name="email" type="email" autocomplete="email" placeholder="vos@ejemplo.com" required></label>
            <label class="field"><span>Contraseña</span><input name="password" type="password" autocomplete="${mode === 'login' ? 'current-password' : 'new-password'}" placeholder="Mínimo 8 caracteres" minlength="8" required></label>
            <div class="form-error hidden"></div>
            <button class="btn btn-primary btn-block" type="submit">${mode === 'login' ? 'Ingresar' : 'Crear cuenta'}</button>
          </form>
          <div class="auth-switch">
            ${mode === 'login' ? '¿Todavía no tenés cuenta?' : '¿Ya tenés una cuenta?'}
            <button type="button" data-action="auth-mode" data-mode="${mode === 'login' ? 'register' : 'login'}">${mode === 'login' ? 'Registrate' : 'Ingresá'}</button>
          </div>
          ${mode === 'login' ? `<div class="auth-demo">
            <strong>Cuenta de analista incluida</strong>
            <code>analyst@trotamundos.dev / analyst12345</code>
            <button type="button" data-action="fill-demo">Completar credenciales</button>
          </div>` : ''}
        </div>
      </section>
    </div>`;
}

function renderShell() {
  const user = state.user || {};
  const canAnalytics = ['analyst', 'admin'].includes(user.role);
  const roleLabel = user.role === 'analyst' ? 'Analista' : user.role === 'admin' ? 'Admin' : 'Viajero';
  app.innerHTML = `
    <div class="shell">
      <aside class="sidebar">
        <a class="brand" href="#/dashboard">
          <span class="mark">${icon('compass', 21)}</span>
          <span>Trotamundos<small>Viajes sin fricción</small></span>
        </a>
        <div class="mobile-brand">
          <span class="mark">${icon('compass', 18)}</span>
          <span>Trotamundos</span>
        </div>
        <nav class="nav">
          <div class="nav-label">Planificación</div>
          ${navItem('dashboard', 'dashboard', 'Resumen')}
          ${navItem('trips', 'map', 'Viajes')}
          ${navItem('shared', 'users', 'Compartidos')}
          <div class="nav-label">Herramientas</div>
          ${navItem('mail', 'mail', 'Correo')}
          ${navItem('agency', 'sparkles', 'Agencia')}
          ${navItem('notifications', 'bell', 'Notificaciones', '<span class="nav-badge hidden" id="nav-notif-badge">0</span>')}
          ${canAnalytics ? `<div class="nav-label">Datos</div>${navItem('analytics', 'chart', 'Analítica')}` : ''}
        </nav>
        <div class="sidebar-foot">
          <div class="user-card">
            <span class="avatar">${esc(initials(user.full_name))}</span>
            <div class="user-meta">
              <strong>${esc(user.full_name || 'Viajero')}</strong>
              <span>${esc(user.email || '')}</span>
            </div>
            <span class="role-chip">${esc(roleLabel)}</span>
          </div>
          <button class="sidebar-logout" type="button" data-action="logout">${icon('logout', 16)} Cerrar sesión</button>
        </div>
      </aside>
      <main class="main">
        <header class="topbar">
          <div class="topbar-left">
            <h1 class="page-title" id="page-title">Resumen</h1>
            <div class="page-sub" id="page-sub">Tu próximo viaje empieza acá</div>
          </div>
          <div class="topbar-actions">
            <a class="icon-btn" href="#/notifications" title="Notificaciones" aria-label="Notificaciones">
              ${icon('bell', 19)}<span class="count hidden" id="bell-count">0</span>
            </a>
            <a class="icon-btn" href="#/mail" title="Correo" aria-label="Correo">${icon('mail', 19)}</a>
          </div>
        </header>
        <div class="content" id="view"></div>
      </main>
    </div>`;
  updateNotifBadge();
}

function navItem(route, iconName, label, extra = '') {
  return `<a class="nav-item" data-nav="${route}" href="#/${route}">${icon(iconName, 18, 'nav-icon')}<span>${label}</span>${extra}</a>`;
}

function setActiveNav(route) {
  document.querySelectorAll('[data-nav]').forEach((el) => {
    el.classList.toggle('active', el.dataset.nav === route);
  });
}

function setChrome(route, title, subtitle) {
  setActiveNav(route);
  const titleEl = document.getElementById('page-title');
  const subEl = document.getElementById('page-sub');
  if (titleEl) titleEl.textContent = title || PAGE_META[route]?.[0] || 'Trotamundos';
  if (subEl) subEl.textContent = subtitle || PAGE_META[route]?.[1] || '';
}

async function renderRoute() {
  if (!state.user) {
    showAuth();
    return;
  }
  const raw = location.hash.replace(/^#\/?/, '') || 'dashboard';
  const parts = raw.split('/');
  const page = parts[0] || 'dashboard';
  state.route = page;
  const view = document.getElementById('view');
  if (!view) {
    renderShell();
    return renderRoute();
  }
  const activeNav = page === 'trips' && parts[1] ? 'trips' : page === 'shared' && parts[1] ? 'shared' : page;
  setChrome(PAGE_META[activeNav] ? activeNav : 'dashboard');
  view.innerHTML = `<div class="loading-block"><span class="spinner"></span><span>Cargando...</span></div>`;
  try {
    if (page === 'dashboard') await renderDashboard();
    else if (page === 'trips' && parts[1]) await renderTripDetail(parts[1], false);
    else if (page === 'trips') await renderTrips();
    else if (page === 'shared' && parts[1]) await renderTripDetail(parts[1], true);
    else if (page === 'shared') await renderShared();
    else if (page === 'mail') await renderMail();
    else if (page === 'agency') await renderAgency();
    else if (page === 'analytics') await renderAnalytics();
    else if (page === 'notifications') await renderNotifications();
    else view.innerHTML = routeErrorHtml(new Error('Página no encontrada'));
  } catch (err) {
    if (err?.status === 401) {
      session.clear();
      showAuth();
      return;
    }
    view.innerHTML = routeErrorHtml(err);
  }
}

function routeErrorHtml(err) {
  return `<div class="card"><div class="card-body">
    ${emptyState('alert', 'No pudimos cargar esta pantalla', friendlyError(err), '<button class="btn btn-primary" type="button" data-action="retry-route">Reintentar</button>')}
  </div></div>`;
}

/* Pages */

async function renderDashboard() {
  await loadNotifications().catch(() => {});
  const [tripsRes, sharedRes] = await Promise.all([api('/trips'), api('/shared-with-me')]);
  const trips = tripsRes.data || [];
  const shared = sharedRes.data || [];
  state.trips.active = trips;
  state.sharedWithMe = shared;
  const reservations = trips.flatMap((t) => t.reservations || []);
  const now = Date.now();
  const upcoming = [...trips]
    .filter((t) => t.end_date && new Date(t.end_date).getTime() >= now)
    .sort((a, b) => new Date(a.start_date || 0) - new Date(b.start_date || 0));
  const next = upcoming[0] || [...trips].sort((a, b) => new Date(b.start_date || 0) - new Date(a.start_date || 0))[0];
  const unread = state.notifications.filter((n) => !n.read_at).length;
  const types = next ? [...new Set((next.reservations || []).map((r) => r.type))] : [];
  const view = document.getElementById('view');
  view.innerHTML = `
    <section class="hero">
      <div>
        <div class="hero-kicker">${next ? 'Tu próximo destino' : 'Empezá a planificar'}</div>
        <h2 class="hero-title">${next ? `Hola, ${esc(firstName(state.user.full_name))}. Te espera <span>${esc(next.destination || next.name)}</span>.` : `Hola, ${esc(firstName(state.user.full_name))}. <span>¿A dónde vamos?</span>`}</h2>
        <div class="hero-meta">
          ${next ? `<span>${icon('calendar', 15)} ${esc(formatRange(next.start_date, next.end_date))}</span>
            <span>${icon('map', 15)} ${esc(next.name || next.destination || 'Viaje')}</span>
            <span>${icon('tag', 15)} ${(next.reservations || []).length} reservas</span>` : `<span>${icon('mail', 15)} Vinculá tu correo y detectá reservas automáticamente</span>
            <span>${icon('sparkles', 15)} Recibí recomendaciones de tu agencia</span>`}
        </div>
        <div class="hero-actions">
          <button class="btn btn-primary" type="button" data-action="new-reservation" ${next ? `data-trip="${esc(next.id)}"` : ''}>${icon('plus', 17)} Nueva reserva</button>
          <a class="btn btn-outline" href="#/trips">${icon('map', 17)} Ver viajes</a>
          <a class="btn btn-outline" href="#/mail">${icon('mail', 17)} Simular correo</a>
        </div>
      </div>
      <div class="hero-side">
        <div class="hero-ticket">
          <div class="trip-kicker">${next ? 'Boarding pass' : 'Tu primer viaje'}</div>
          <div class="ticket-dest">${esc(next?.destination || 'Un nuevo destino')}</div>
          <div class="ticket-dates">${next ? esc(formatRange(next.start_date, next.end_date)) : 'Las fechas las ponés vos'}</div>
          <div class="ticket-row">
            ${types.length ? types.map((t) => `<span class="ticket-chip">${esc(typeLabel(t))}</span>`).join('') : '<span class="ticket-chip">Sin reservas todavía</span>'}
          </div>
        </div>
      </div>
    </section>

    <section class="stat-grid">
      <div class="stat">
        <div class="stat-top"><span class="stat-icon">${icon('map', 18)}</span><span class="pill">${trips.length}</span></div>
        <div class="stat-value">${trips.length}</div>
        <div class="stat-label">Viajes activos</div>
      </div>
      <div class="stat">
        <div class="stat-top"><span class="stat-icon coral">${icon('tag', 18)}</span><span class="pill">${reservations.filter((r) => r.status === 'confirmed').length} confirmadas</span></div>
        <div class="stat-value">${reservations.length}</div>
        <div class="stat-label">Reservas totales</div>
      </div>
      <div class="stat">
        <div class="stat-top"><span class="stat-icon gold">${icon('bell', 18)}</span><span class="pill">${unread ? 'Pendientes' : 'Al día'}</span></div>
        <div class="stat-value">${unread}</div>
        <div class="stat-label">Notificaciones sin leer</div>
      </div>
      <div class="stat">
        <div class="stat-top"><span class="stat-icon blue">${icon('users', 18)}</span><span class="pill">Solo lectura</span></div>
        <div class="stat-value">${shared.length}</div>
        <div class="stat-label">Viajes compartidos con vos</div>
      </div>
    </section>

    <section>
      <div class="section-head">
        <div>
          <h2 class="section-title">Tus viajes</h2>
          <p class="section-sub">Agrupados por destino y fechas. Se actualizan solos.</p>
        </div>
        <a class="btn btn-outline btn-sm" href="#/trips">Ver todos ${icon('arrowRight', 15)}</a>
      </div>
      <div class="trip-grid" style="margin-top:14px">
        ${trips.length ? trips.slice(0, 3).map(tripCard).join('') : emptyState('compass', 'Todavía no hay viajes', 'Creá una reserva manual o vinculá tu correo para que detectemos la primera.', '<button class="btn btn-primary btn-sm" type="button" data-action="new-reservation">Crear primera reserva</button>')}
      </div>
    </section>

    <div class="grid grid-2">
      <section class="card">
        <div class="card-head">
          <div><div class="card-title">Notificaciones recientes</div><div class="card-sub">Cambios de reservas y novedades</div></div>
          <a class="btn btn-ghost btn-sm" href="#/notifications">Ver todas</a>
        </div>
        <div class="card-body" style="padding-top:12px">
          ${state.notifications.length ? state.notifications.slice(0, 4).map(notificationRow).join('') : '<div class="muted small" style="padding:12px 0">Sin novedades por ahora.</div>'}
        </div>
      </section>
      <section class="card">
        <div class="card-head">
          <div><div class="card-title">Atajos de prueba</div><div class="card-sub">Todo lo que podés probar desde acá</div></div>
        </div>
        <div class="card-body">
          <div class="stack">
            <a class="share-row" href="#/mail"><span class="row">${icon('mail', 17)} Simular un email de reserva</span>${icon('arrowRight', 16)}</a>
            <a class="share-row" href="#/agency"><span class="row">${icon('sparkles', 17)} Elegir agencia favorita</span>${icon('arrowRight', 16)}</a>
            <a class="share-row" href="#/shared"><span class="row">${icon('users', 17)} Probar compartición de viajes</span>${icon('arrowRight', 16)}</a>
            ${['analyst', 'admin'].includes(state.user.role) ? `<a class="share-row" href="#/analytics"><span class="row">${icon('chart', 17)} Ver analítica anonimizada</span>${icon('arrowRight', 16)}</a>` : ''}
          </div>
        </div>
      </section>
    </div>`;
}

function tripCard(trip) {
  const reservations = trip.reservations || [];
  const types = [...new Set(reservations.map((r) => r.type))].slice(0, 4);
  const search = [trip.name, trip.destination, trip.status].join(' ').toLowerCase();
  return `<a class="trip-card" href="#/trips/${esc(trip.id)}" data-trip-card data-search="${esc(search)}">
    <div class="trip-card-top">
      <div>
        <div class="trip-kicker">${trip.status === 'archived' ? 'Viaje archivado' : 'Próximo viaje'}</div>
        <div class="trip-destination">${esc(trip.destination || trip.name || 'Sin destino')}</div>
        <div class="trip-name">${esc(trip.name || '')}</div>
      </div>
      ${statusBadge(trip.status)}
    </div>
    <div class="trip-dates">${icon('calendar', 15)} ${esc(formatRange(trip.start_date, trip.end_date))}</div>
    <div class="trip-foot">
      <span class="muted small">${reservations.length} ${reservations.length === 1 ? 'reserva' : 'reservas'}</span>
      <span class="type-dots">${types.length ? types.map((t) => `<span class="type-dot ${esc(t)}">${icon(typeIcon(t), 14)}</span>`).join('') : `<span class="type-dot">${icon('tag', 14)}</span>`}</span>
    </div>
  </a>`;
}

async function renderTrips() {
  const [activeRes, archivedRes] = await Promise.all([api('/trips'), api('/trips?archived=true')]);
  state.trips.active = activeRes.data || [];
  state.trips.archived = archivedRes.data || [];
  const tab = state.trips.tab;
  const trips = tab === 'archived' ? state.trips.archived : state.trips.active;
  const view = document.getElementById('view');
  view.innerHTML = `
    <div class="toolbar">
      <div class="tabs">
        <button class="tab ${tab === 'active' ? 'active' : ''}" type="button" data-action="trip-tab" data-tab="active">Activos (${state.trips.active.length})</button>
        <button class="tab ${tab === 'archived' ? 'active' : ''}" type="button" data-action="trip-tab" data-tab="archived">Archivados (${state.trips.archived.length})</button>
      </div>
      <div class="row">
        <div class="search">${icon('search', 16)}<input id="trip-search" placeholder="Buscar destino o viaje" value="${esc(state.trips.query)}"></div>
        <button class="btn btn-primary" type="button" data-action="new-reservation">${icon('plus', 17)} Nueva reserva</button>
      </div>
    </div>
    ${trips.length ? `<div class="trip-grid" style="margin-top:18px">${trips.map(tripCard).join('')}</div>` : emptyState(
      tab === 'archived' ? 'archive' : 'compass',
      tab === 'archived' ? 'No hay viajes archivados' : 'Todavía no hay viajes',
      tab === 'archived' ? 'Cuando un viaje termina se archiva automáticamente para mantener tu lista ordenada.' : 'Creá una reserva manual o simulá un correo para que armemos tu primer itinerario.',
      tab === 'archived' ? '' : '<button class="btn btn-primary btn-sm" type="button" data-action="new-reservation">Crear reserva</button>',
    )}`;
  applyTripSearch();
}

function applyTripSearch() {
  const q = state.trips.query.trim().toLowerCase();
  document.querySelectorAll('[data-trip-card]').forEach((card) => {
    card.classList.toggle('hidden', q.length > 0 && !card.dataset.search.includes(q));
  });
}

async function renderTripDetail(id, shared = false) {
  let trip;
  let shares = [];
  let sharedMeta = null;
  if (shared) {
    const payload = await api(`/shared/${id}`);
    trip = payload.trip;
    sharedMeta = payload.share;
  } else {
    const [tripData, shareData] = await Promise.all([api(`/trips/${id}`), api(`/trips/${id}/shares`)]);
    trip = tripData;
    shares = shareData.data || [];
  }
  state.currentTrip = trip;
  state.currentShares = shares;
  const reservations = [...(trip.reservations || [])].sort((a, b) => new Date(a.start_at || 0) - new Date(b.start_at || 0));
  const view = document.getElementById('view');
  const back = shared ? '#/shared' : '#/trips';
  setChrome(shared ? 'shared' : 'trips');
  document.getElementById('page-title').textContent = shared ? 'Viaje compartido' : (trip.destination || trip.name || 'Detalle del viaje');
  document.getElementById('page-sub').textContent = shared ? 'Vista de solo lectura' : 'Reservas, compartición y redes';
  view.innerHTML = `
    <div class="row-between">
      <a class="btn btn-ghost btn-sm" href="${back}">${icon('arrowLeft', 16)} Volver</a>
      <div class="detail-actions">
        ${shared ? '' : `<button class="btn btn-outline" type="button" data-action="new-reservation" data-trip="${esc(trip.id)}">${icon('plus', 16)} <span>Reserva</span></button>
        <button class="btn btn-outline" type="button" data-action="share-trip" data-id="${esc(trip.id)}">${icon('share', 16)} <span>Compartir</span></button>
        <button class="btn btn-outline" type="button" data-action="social-trip" data-id="${esc(trip.id)}">${icon('network', 16)} <span>Redes</span></button>
        <button class="btn btn-outline" type="button" data-action="location-trip" data-id="${esc(trip.id)}">${icon('location', 16)} <span>Contexto</span></button>
        ${trip.status === 'archived' ? `<button class="btn btn-danger" type="button" data-action="delete-trip" data-id="${esc(trip.id)}">${icon('trash', 16)}</button>` : ''}`}
      </div>
    </div>

    <section class="card">
      <div class="card-body">
        <div class="detail-head" style="border-bottom:0;padding-bottom:0">
          <div>
            <div class="trip-kicker">${shared ? 'Compartido con vos' : 'Itinerario'}</div>
            <h2 class="detail-title">${esc(trip.destination || trip.name || 'Viaje')}</h2>
            <div class="detail-meta">
              <span>${icon('calendar', 15)} ${esc(formatRange(trip.start_date, trip.end_date))}</span>
              <span>${icon('map', 15)} ${esc(trip.name || '')}</span>
              <span>${icon('tag', 15)} ${reservations.length} reservas</span>
              ${statusBadge(trip.status)}
            </div>
          </div>
          ${shared ? `<div class="pill">${icon('shield', 14)} Solo lectura</div>` : ''}
        </div>
        ${shared && sharedMeta ? `<div class="divider"></div><div class="hint">${icon('info', 14)} Este viaje fue compartido por otro viajero. Podés consultar el itinerario, pero no modificarlo.</div>` : ''}
      </div>
    </section>

    <div class="grid grid-2">
      <section class="card">
        <div class="card-head">
          <div><div class="card-title">Reservas</div><div class="card-sub">Ordenadas por fecha de inicio</div></div>
          ${shared ? '' : `<button class="btn btn-ghost btn-sm" type="button" data-action="new-reservation" data-trip="${esc(trip.id)}">${icon('plus', 15)} Agregar</button>`}
        </div>
        <div class="card-body" style="padding-top:13px">
          ${reservations.length ? `<div class="reservation-list">${reservations.map((r) => reservationCard(r, shared)).join('')}</div>` : emptyState('tag', 'Sin reservas todavía', shared ? 'El dueño del viaje no cargó reservas.' : 'Agregá una reserva manual o esperá la ingesta de correos.')}
        </div>
      </section>

      <div class="stack">
        ${shared ? `<section class="card"><div class="card-head"><div><div class="card-title">Sobre este viaje</div><div class="card-sub">Información de la compartición</div></div></div><div class="card-body"><dl class="kv">
          <dt>Compartido con</dt><dd>${esc(sharedMeta?.guest_email || state.user.email)}</dd>
          <dt>Fecha</dt><dd>${esc(formatDateTime(sharedMeta?.created_at))}</dd>
          <dt>Permiso</dt><dd>Solo lectura</dd>
        </dl></div></section>` : `
        <section class="card">
          <div class="card-head">
            <div><div class="card-title">Compartir viaje</div><div class="card-sub">Acceso de solo lectura por email</div></div>
            <button class="btn btn-ghost btn-sm" type="button" data-action="share-trip" data-id="${esc(trip.id)}">${icon('plus', 15)} Invitar</button>
          </div>
          <div class="card-body" style="padding-top:13px">
            ${shares.length ? `<div class="share-list">${shares.map((s) => `<div class="share-row"><span>${esc(s.guest_email)}</span><button class="btn-icon danger" type="button" title="Revocar acceso" data-action="revoke-share" data-id="${esc(s.id)}">${icon('trash', 14)}</button></div>`).join('')}</div>` : '<div class="muted small">Todavía no compartiste este viaje con nadie.</div>'}
          </div>
        </section>
        <section class="card">
          <div class="card-head"><div><div class="card-title">Acciones rápidas</div><div class="card-sub">Todo lo que podés hacer con este viaje</div></div></div>
          <div class="card-body" style="padding-top:12px">
            <div class="stack">
              <button class="share-row" type="button" data-action="social-trip" data-id="${esc(trip.id)}" style="border:0;background:none;width:100%;text-align:left;cursor:pointer"><span class="row">${icon('network', 17)} Publicar en redes</span>${icon('arrowRight', 15)}</button>
              <button class="share-row" type="button" data-action="location-trip" data-id="${esc(trip.id)}" style="border:0;background:none;width:100%;text-align:left;cursor:pointer"><span class="row">${icon('location', 17)} Ver distancia al destino</span>${icon('arrowRight', 15)}</button>
            </div>
          </div>
        </section>`}
      </div>
    </div>`;
}

function reservationCard(r, readOnly = false) {
  return `<article class="reservation">
    <div class="res-type ${esc(r.type)}">${icon(typeIcon(r.type), 19)}</div>
    <div>
      <div class="res-title">${esc(r.title || typeLabel(r.type))} ${r.stale ? '<span class="badge badge-amber">Desactualizada</span>' : ''} ${statusBadge(r.status)}</div>
      <div class="res-meta">
        <span>${icon('calendar', 13)} ${esc(formatRange(r.start_at, r.end_at))}</span>
        ${r.provider ? `<span>${icon('globe', 13)} ${esc(r.provider)}</span>` : ''}
        ${r.provider_locator ? `<span>${icon('tag', 13)} <span class="code">${esc(r.provider_locator)}</span></span>` : ''}
        <span>${icon('info', 13)} ${esc(sourceLabel(r.source))}</span>
        ${r.last_synced_at ? `<span>${icon('sync', 13)} ${esc(relativeTime(r.last_synced_at))}</span>` : ''}
      </div>
      ${r.stale ? `<div class="stale-note">${icon('alert', 13)} No pudimos actualizar con el proveedor. Mostramos el último dato conocido.</div>` : ''}
    </div>
    <div class="res-side">
      ${readOnly ? '' : `<div class="res-actions">
        <button class="btn-icon" type="button" title="Sincronizar proveedor" data-action="sync-reservation" data-id="${esc(r.id)}">${icon('sync', 14)}</button>
        <button class="btn-icon" type="button" title="Editar reserva" data-action="edit-reservation" data-id="${esc(r.id)}">${icon('edit', 14)}</button>
        <button class="btn-icon" type="button" title="Historial de cambios" data-action="changes-reservation" data-id="${esc(r.id)}">${icon('history', 14)}</button>
        <button class="btn-icon" type="button" title="Consultar agencia" data-action="contact-reservation" data-id="${esc(r.id)}">${icon('send', 14)}</button>
        <button class="btn-icon danger" type="button" title="Eliminar reserva" data-action="delete-reservation" data-id="${esc(r.id)}">${icon('trash', 14)}</button>
      </div>`}
    </div>
  </article>`;
}

async function renderShared() {
  const [sharedRes, activeRes, archivedRes] = await Promise.all([
    api('/shared-with-me'),
    api('/trips'),
    api('/trips?archived=true'),
  ]);
  state.sharedWithMe = sharedRes.data || [];
  const allTrips = [...(activeRes.data || []), ...(archivedRes.data || [])];
  const perTrip = await Promise.all(allTrips.map((trip) =>
    api(`/trips/${trip.id}/shares`).then((res) => ({ trip, shares: res.data || [] })).catch(() => ({ trip, shares: [] }))
  ));
  state.sharedSent = perTrip.flatMap(({ trip, shares }) => shares.map((share) => ({ trip, share })));
  const view = document.getElementById('view');
  view.innerHTML = `
    <div class="section-head">
      <div><h2 class="section-title">Compartidos con vos</h2><p class="section-sub">Viajes que otros usuarios compartieron con tu email.</p></div>
      <span class="pill">${state.sharedWithMe.length} viajes</span>
    </div>
    ${state.sharedWithMe.length ? `<div class="trip-grid" style="margin-top:14px">${state.sharedWithMe.map(({ share, trip }) => `
      <a class="trip-card" href="#/shared/${esc(trip.id)}">
        <div class="trip-card-top">
          <div><div class="trip-kicker">Compartido con vos</div><div class="trip-destination">${esc(trip.destination || trip.name)}</div><div class="trip-name">${esc(trip.name || '')}</div></div>
          <span class="badge badge-teal">Solo lectura</span>
        </div>
        <div class="trip-dates">${icon('calendar', 15)} ${esc(formatRange(trip.start_date, trip.end_date))}</div>
        <div class="trip-foot"><span class="muted small">${(trip.reservations || []).length} reservas</span><span class="muted small">${esc(formatDateTime(share.created_at))}</span></div>
      </a>`).join('')}</div>` : `<div style="margin-top:14px">${emptyState('users', 'No tenés viajes compartidos', 'Cuando alguien comparta un itinerario con tu email, va a aparecer acá automáticamente.')}</div>`}

    <div class="section-head" style="margin-top:26px">
      <div><h2 class="section-title">Compartidos por vos</h2><p class="section-sub">Personas con acceso de solo lectura a tus viajes.</p></div>
      <span class="pill">${state.sharedSent.length} accesos</span>
    </div>
    <section class="card" style="margin-top:14px">
      <div class="card-body" style="padding-top:12px">
        ${state.sharedSent.length ? `<div class="share-list">${state.sharedSent.map(({ trip, share }) => `<div class="share-row">
          <div><strong>${esc(share.guest_email)}</strong><div class="muted tiny">${esc(trip.destination || trip.name)} · ${esc(formatDateTime(share.created_at))}</div></div>
          <div class="row"><a class="btn btn-ghost btn-sm" href="#/trips/${esc(trip.id)}">Ver viaje</a><button class="btn-icon danger" type="button" title="Revocar" data-action="revoke-share" data-id="${esc(share.id)}">${icon('trash', 14)}</button></div>
        </div>`).join('')}</div>` : '<div class="muted small">Todavía no compartiste viajes.</div>'}
      </div>
    </section>`;
}

async function renderMail() {
  const [accounts, events, whitelist, filters] = await Promise.all([
    api('/mail/accounts'), api('/mail/events?limit=50'), api('/mail/whitelist'), api('/mail/filters'),
  ]);
  state.mail = {
    accounts: accounts.data || [],
    events: events.data || [],
    whitelist: whitelist.data || [],
    filters: filters.data || [],
  };
  const account = state.mail.accounts.find((a) => a.status === 'active') || state.mail.accounts[0];
  const defaultEmail = account?.email || state.user.email;
  const locator = `WEB${String(Date.now()).slice(-6)}`;
  const start = new Date(Date.now() + 45 * 86400000).toISOString().slice(0, 10);
  const end = new Date(Date.now() + 52 * 86400000).toISOString().slice(0, 10);
  const view = document.getElementById('view');
  view.innerHTML = `
    <div class="section-head">
      <div><h2 class="section-title">Ingesta de correo</h2><p class="section-sub">Vinculá una casilla mock, simulá un email y mirá cómo se procesa.</p></div>
      <button class="btn btn-outline btn-sm" type="button" data-action="refresh-view">${icon('refresh', 15)} Actualizar</button>
    </div>

    <div class="mail-grid">
      <div class="stack">
        <section class="card">
          <div class="card-head"><div><div class="card-title">Cuentas vinculadas</div><div class="card-sub">Solo permiso de lectura</div></div><span class="pill">${state.mail.accounts.length}</span></div>
          <div class="card-body" style="padding-top:13px">
            ${state.mail.accounts.length ? state.mail.accounts.map((acc) => `<div class="account-card">
              <div class="row-between">
                <div><div class="account-mail">${esc(acc.email)}</div><div class="account-meta">${esc(acc.provider)} · ${esc(acc.status)} · ${acc.last_scanned_at ? `último escaneo ${esc(relativeTime(acc.last_scanned_at))}` : 'nunca escaneada'}</div></div>
                <div class="row">${statusBadge(acc.status === 'active' ? 'active' : 'canceled')}<button class="btn-icon" type="button" title="Escanear" data-action="scan-mailbox" data-id="${esc(acc.id)}" ${acc.status !== 'active' ? 'disabled' : ''}>${icon('refresh', 14)}</button><button class="btn-icon danger" type="button" title="Revocar" data-action="revoke-mailbox" data-id="${esc(acc.id)}">${icon('trash', 14)}</button></div>
              </div>
            </div>`).join('') : '<div class="muted small" style="margin-bottom:12px">No hay cuentas vinculadas.</div>'}
            <div class="divider"></div>
            <form class="form-grid" data-form="mail-account">
              <label class="field"><span>Proveedor</span><select name="provider"><option value="mock">Mock</option><option value="gmail">Gmail</option><option value="outlook">Outlook</option></select></label>
              <label class="field"><span>Email</span><input name="email" type="email" placeholder="${esc(state.user.email)}" required></label>
              <label class="field full"><span>Access token de prueba</span><input name="access_token" value="demo-token" required></label>
              <div class="form-actions full"><button class="btn btn-primary btn-sm" type="submit">${icon('link', 15)} Vincular cuenta</button></div>
            </form>
          </div>
        </section>

        <section class="card">
          <div class="card-head"><div><div class="card-title">Simulador de email</div><div class="card-sub">Agrega un mensaje a la bandeja mock y escanealo</div></div></div>
          <div class="card-body" style="padding-top:13px">
            <form class="stack" data-form="mock-mail">
              <div class="form-grid">
                <label class="field"><span>Bandeja</span><input name="email" type="email" value="${esc(defaultEmail)}" required></label>
                <label class="field"><span>Remitente</span><input name="from" value="reservas@aerolineas.example" required></label>
                <label class="field full"><span>Asunto</span><input name="subject" value="Tu vuelo a Bariloche está confirmado" required></label>
                <label class="field full"><span>Cuerpo</span><textarea name="body" rows="7" required>Provider: aerolineas
Locator: ${locator}
Type: flight
Title: Vuelo a Bariloche
Destination: Bariloche
Start: ${start}T08:00:00Z
End: ${start}T10:00:00Z</textarea></label>
                <label class="field"><span>Clave mock</span><input name="mock_key" value="${esc(state.mockKey)}" required></label>
                <label class="checkbox" style="align-self:end;padding-bottom:10px"><input type="checkbox" name="scan_after" checked> Escanear después de agregar</label>
              </div>
              <div class="form-actions between" style="margin-top:2px">
                <span class="hint">Se usa el header <span class="code">X-Mock-Key</span> para los endpoints mock.</span>
                <button class="btn btn-coral btn-sm" type="submit">${icon('send', 15)} Agregar y procesar</button>
              </div>
            </form>
          </div>
        </section>
      </div>

      <section class="card">
        <div class="card-head"><div><div class="card-title">Eventos de ingesta</div><div class="card-sub">Los últimos 50 eventos del outbox</div></div><span class="pill">${state.mail.events.length}</span></div>
        <div class="card-body" style="padding-top:10px">
          ${state.mail.events.length ? `<div class="table-wrap"><table class="event-table">
            <thead><tr><th>Estado</th><th>Asunto</th><th>Remitente</th><th>Recibido</th><th>Intentos</th></tr></thead>
            <tbody>${state.mail.events.map((ev) => `<tr>
              <td>${statusBadge(ev.status)}</td>
              <td><div class="event-subject" title="${esc(ev.payload?.subject || ev.message_id)}">${esc(ev.payload?.subject || ev.message_id)}</div>${ev.last_error ? `<div class="event-error">${esc(ev.last_error)}</div>` : ''}</td>
              <td class="small">${esc(ev.payload?.from || '—')}</td>
              <td class="small nowrap">${esc(ev.received_at ? formatDateTime(ev.received_at) : formatDateTime(ev.created_at))}</td>
              <td class="small">${esc(ev.attempts)}</td>
            </tr>`).join('')}</tbody>
          </table></div>` : emptyState('mail', 'No hay eventos todavía', 'Vinculá una cuenta y agregá un email de prueba para ver el flujo completo.')}
        </div>
      </section>
    </div>

    <div class="grid grid-2">
      <section class="card">
        <div class="card-head"><div><div class="card-title">Whitelist de remitentes</div><div class="card-sub">Si está vacía, se aceptan todos</div></div><span class="pill">${state.mail.whitelist.length}</span></div>
        <div class="card-body" style="padding-top:13px">
          ${state.mail.whitelist.length ? state.mail.whitelist.map((w) => `<div class="rule-row"><span class="code">${esc(w.pattern)}</span><button class="btn-icon danger" type="button" title="Eliminar" data-action="delete-whitelist" data-id="${esc(w.id)}">${icon('trash', 13)}</button></div>`).join('') : '<div class="muted small" style="margin-bottom:8px">Sin reglas: se procesan todos los remitentes.</div>'}
          <form class="inline-form" data-form="whitelist" style="margin-top:11px">
            <label class="field"><span>Patrón</span><input name="pattern" placeholder="*@aerolineas.com" required></label>
            <button class="btn btn-outline btn-sm" type="submit">${icon('plus', 14)} Agregar</button>
          </form>
        </div>
      </section>

      <section class="card">
        <div class="card-head"><div><div class="card-title">Filtros de correo</div><div class="card-sub">Todos los filtros activos deben cumplirse</div></div><span class="pill">${state.mail.filters.length}</span></div>
        <div class="card-body" style="padding-top:13px">
          ${state.mail.filters.length ? state.mail.filters.map((f) => `<div class="rule-row"><span><span class="code">${esc(f.field)} ${esc(f.operator)}</span> ${esc(f.value)}</span><span class="row">${statusBadge(f.active ? 'active' : 'ignored')}<button class="btn-icon" type="button" title="Activar/desactivar" data-action="toggle-filter" data-id="${esc(f.id)}">${icon('refresh', 13)}</button><button class="btn-icon danger" type="button" title="Eliminar" data-action="delete-filter" data-id="${esc(f.id)}">${icon('trash', 13)}</button></span></div>`).join('') : '<div class="muted small" style="margin-bottom:8px">Sin filtros: se procesan todos los emails que pasan la whitelist.</div>'}
          <form class="form-grid" data-form="filter" style="margin-top:12px">
            <label class="field"><span>Campo</span><select name="field"><option value="subject">Asunto</option><option value="body">Cuerpo</option><option value="sender">Remitente</option></select></label>
            <label class="field"><span>Operador</span><select name="operator"><option value="contains">Contiene</option><option value="not_contains">No contiene</option><option value="equals">Igual a</option><option value="starts_with">Empieza con</option></select></label>
            <label class="field full"><span>Valor</span><input name="value" placeholder="vuelo" required></label>
            <label class="checkbox full"><input type="checkbox" name="active" checked> Activo</label>
            <div class="form-actions full"><button class="btn btn-outline btn-sm" type="submit">${icon('plus', 14)} Agregar filtro</button></div>
          </form>
        </div>
      </section>
    </div>`;
}

async function renderAgency() {
  const [agencies, favorite, recommendations, requests] = await Promise.all([
    api('/agencies'), api('/agencies/favorite'), api('/recommendations'), api('/agency-requests'),
  ]);
  state.agencies = agencies.data || [];
  state.favorite = favorite.favorite;
  state.recommendations = recommendations.data || [];
  state.requests = requests.data || [];
  const fav = state.favorite;
  const agency = fav?.agency;
  const view = document.getElementById('view');
  view.innerHTML = `
    <div class="section-head">
      <div><h2 class="section-title">Tu agencia de confianza</h2><p class="section-sub">Elegí una agencia para recibir propuestas y hacer consultas.</p></div>
      ${fav ? `<span class="pill gold">${icon('star', 14)} ${esc(agency?.name || 'Favorita')}</span>` : `<span class="pill">Sin favorita</span>`}
    </div>

    ${fav ? `<section class="card" style="margin-top:14px"><div class="card-body">
      <div class="row-between">
        <div class="row" style="align-items:flex-start">
          <span class="stat-icon gold" style="width:46px;height:46px">${icon('star', 20)}</span>
          <div><div class="agency-name">${esc(agency?.name || 'Agencia favorita')}</div><div class="agency-desc">${esc(agency?.description || '')}</div><div class="agency-contact" style="margin-top:5px">${esc(agency?.email || '')} ${agency?.phone ? `· ${esc(agency.phone)}` : ''}</div></div>
        </div>
        <button class="btn btn-outline btn-sm" type="button" data-action="remove-favorite">${icon('x', 14)} Quitar favorita</button>
      </div>
    </div></section>` : ''}

    ${!fav ? `<section style="margin-top:18px"><div class="agency-grid">${state.agencies.map((a) => `<article class="agency-card">
      <div class="row-between"><div class="agency-name">${esc(a.name)}</div><span class="stat-icon gold">${icon('sparkles', 17)}</span></div>
      <div class="agency-desc">${esc(a.description)}</div>
      <div class="agency-contact">${esc(a.email)}${a.phone ? ` · ${esc(a.phone)}` : ''}</div>
      <button class="btn btn-primary btn-sm" type="button" data-action="set-favorite" data-id="${esc(a.id)}">${icon('star', 14)} Elegir como favorita</button>
    </article>`).join('')}</div></section>` : `
    <section style="margin-top:22px">
      <div class="section-head"><div><h2 class="section-title">Recomendaciones</h2><p class="section-sub">Propuestas activas de tu agencia favorita.</p></div><span class="pill">${state.recommendations.length}</span></div>
      ${state.recommendations.length ? `<div class="grid grid-2" style="margin-top:14px">${state.recommendations.map((rec) => `<article class="recommendation">
        <div class="rec-top"><div><div class="rec-title">${esc(rec.title)}</div><div class="muted small">${esc(rec.destination)} · ${esc(formatRange(rec.start_date, rec.end_date))}</div></div><span class="price">${esc(formatMoney(rec.price_cents, rec.currency))}</span></div>
        <div class="rec-desc">${esc(rec.description)}</div>
        <div class="row"><button class="btn btn-coral btn-sm" type="button" data-action="inquire-rec" data-id="${esc(rec.id)}">${icon('send', 14)} Consultar</button><button class="btn btn-ghost btn-sm" type="button" data-action="discard-rec" data-id="${esc(rec.id)}">Descartar</button></div>
      </article>`).join('')}</div>` : `<div style="margin-top:14px">${emptyState('sparkles', 'No hay recomendaciones activas', 'Cuando tu agencia publique nuevas propuestas van a aparecer acá.')}</div>`}
    </section>`}

    <section class="card" style="margin-top:22px">
      <div class="card-head"><div><div class="card-title">Consultas enviadas</div><div class="card-sub">Seguimiento de tus pedidos a la agencia</div></div><span class="pill">${state.requests.length}</span></div>
      <div class="card-body" style="padding-top:12px">
        ${state.requests.length ? state.requests.map((req) => `<div class="request-row">
          <div><div class="row"><strong>${esc(req.agency_name || 'Agencia')}</strong>${statusBadge(req.status)}</div><div class="request-msg">${esc(req.message || 'Sin mensaje')}</div><div class="muted tiny" style="margin-top:4px">${esc(formatDateTime(req.created_at))}${req.context?.title ? ` · ${esc(req.context.title)}` : ''}${req.context?.destination ? ` · ${esc(req.context.destination)}` : ''}</div></div>
          ${req.status === 'open' ? `<button class="btn btn-ghost btn-sm" type="button" data-action="close-request" data-id="${esc(req.id)}">Cerrar</button>` : ''}
        </div>`).join('') : '<div class="muted small">Todavía no enviaste consultas.</div>'}
      </div>
    </section>`;
}

async function renderAnalytics() {
  if (!['analyst', 'admin'].includes(state.user.role)) {
    document.getElementById('view').innerHTML = emptyState('shield', 'Sección reservada para analistas', 'Ingresá con la cuenta analyst@trotamundos.dev para ver métricas anonimizadas.', '<a class="btn btn-primary btn-sm" href="#/dashboard">Volver al resumen</a>');
    return;
  }
  const { from, to } = state.analyticsRange;
  const query = new URLSearchParams({ from: `${from}T00:00:00Z`, to: `${to}T23:59:59Z` });
  const [summary, facts] = await Promise.all([
    api(`/analytics/summary?${query}`),
    api(`/analytics/facts?${query}`),
  ]);
  const top = summary.top_destinations?.[0]?.key || '—';
  const view = document.getElementById('view');
  view.innerHTML = `
    <div class="section-head">
      <div><h2 class="section-title">Analítica anonimizada</h2><p class="section-sub">Sin usuarios, emails ni localizadores. Solo datos agregados.</p></div>
      <form class="range-form" data-form="analytics-range">
        <label class="field"><span>Desde</span><input type="date" name="from" value="${esc(from)}" required></label>
        <label class="field"><span>Hasta</span><input type="date" name="to" value="${esc(to)}" required></label>
        <button class="btn btn-primary btn-sm" type="submit">${icon('chart', 15)} Aplicar</button>
      </form>
    </div>

    <section class="kpi-grid" style="margin-top:20px">
      <div class="kpi"><div class="kpi-label">Viajes</div><div class="kpi-value">${summary.trips}</div><div class="kpi-foot">destinos distintos en el período</div></div>
      <div class="kpi"><div class="kpi-label">Reservas</div><div class="kpi-value">${summary.reservations}</div><div class="kpi-foot">hechos anonimizados</div></div>
      <div class="kpi"><div class="kpi-label">Canceladas</div><div class="kpi-value">${summary.canceled}</div><div class="kpi-foot">reservas con estado cancelado</div></div>
      <div class="kpi"><div class="kpi-label">Destino top</div><div class="kpi-value" style="font-size:22px">${esc(top)}</div><div class="kpi-foot">mayor volumen de reservas</div></div>
    </section>

    <div class="grid grid-2" style="margin-top:20px">
      <section class="card"><div class="card-head"><div><div class="card-title">Por tipo</div><div class="card-sub">Vuelos, alojamientos y autos</div></div></div><div class="card-body" style="padding-top:14px">${bars((summary.by_type || []).map((b) => ({ key: typeLabel(b.key), count: b.count })))}</div></section>
      <section class="card"><div class="card-head"><div><div class="card-title">Por proveedor</div><div class="card-sub">Volumen por adapter</div></div></div><div class="card-body" style="padding-top:14px">${bars(summary.by_provider || [])}</div></section>
      <section class="card"><div class="card-head"><div><div class="card-title">Destinos top</div><div class="card-sub">Hasta 10 destinos</div></div></div><div class="card-body" style="padding-top:14px">${bars(summary.top_destinations || [])}</div></section>
      <section class="card"><div class="card-head"><div><div class="card-title">Evolución mensual</div><div class="card-sub">Reservas por mes</div></div></div><div class="card-body" style="padding-top:14px">${bars(summary.monthly || [])}</div></section>
    </div>

    <section class="card" style="margin-top:20px">
      <div class="card-head"><div><div class="card-title">Hechos recientes</div><div class="card-sub">Máximo 100 registros del período</div></div><span class="pill">${(facts.data || []).length}</span></div>
      <div class="card-body" style="padding-top:10px">
        ${(facts.data || []).length ? `<div class="table-wrap"><table class="event-table">
          <thead><tr><th>Destino</th><th>Tipo</th><th>Proveedor</th><th>Estado</th><th>Ocurrido</th></tr></thead>
          <tbody>${facts.data.map((f) => `<tr><td>${esc(f.destination || '—')}</td><td>${esc(typeLabel(f.type))}</td><td>${esc(f.provider || '—')}</td><td>${statusBadge(f.status)}</td><td class="small nowrap">${esc(formatDateTime(f.occurred_at))}</td></tr>`).join('')}</tbody>
        </table></div>` : emptyState('chart', 'Sin hechos en el período', 'Probá ampliar el rango de fechas o generá nuevas reservas.')}
      </div>
    </section>`;
}

async function renderNotifications() {
  await loadNotifications();
  const view = document.getElementById('view');
  const unread = state.notifications.filter((n) => !n.read_at);
  view.innerHTML = `
    <div class="section-head">
      <div><h2 class="section-title">Notificaciones</h2><p class="section-sub">${unread.length ? `${unread.length} sin leer` : 'Estás al día'}</p></div>
      ${unread.length ? `<button class="btn btn-outline btn-sm" type="button" data-action="mark-all-read">${icon('check', 15)} Marcar todas como leídas</button>` : ''}
    </div>
    <section class="card" style="margin-top:14px"><div class="card-body" style="padding-top:6px">
      ${state.notifications.length ? state.notifications.map((n) => notificationRow(n, true)).join('') : emptyState('bell', 'Sin notificaciones', 'Las novedades de tus reservas y viajes van a aparecer acá.')}
    </div></section>`;
}

function notificationRow(n, actions = false) {
  return `<div class="notification-row">
    <span class="notification-dot ${n.read_at ? 'read' : ''}"></span>
    <div style="min-width:0;flex:1">
      <div class="notification-title">${esc(n.title)}</div>
      <div class="notification-body">${esc(n.body)}</div>
      <div class="notification-time">${esc(relativeTime(n.created_at))}${n.resource_type ? ` · ${esc(n.resource_type)}` : ''}</div>
    </div>
    ${actions && !n.read_at ? `<button class="btn btn-ghost btn-sm" type="button" data-action="mark-read" data-id="${esc(n.id)}">Marcar leída</button>` : ''}
  </div>`;
}

/* Modals */

function openReservationModal(reservation, tripId = '') {
  const editing = Boolean(reservation);
  const r = reservation || { type: 'flight', status: 'confirmed', details: {} };
  openModal({
    title: editing ? 'Editar reserva' : 'Nueva reserva',
    subtitle: editing ? 'Los cambios se reflejan en el itinerario y la analítica.' : 'Si no elegís un viaje, lo agrupamos automáticamente por destino y fechas.',
    wide: true,
    body: `<form data-form="reservation" class="stack">
      <input type="hidden" name="id" value="${esc(r.id || '')}">
      <input type="hidden" name="trip_id" value="${esc(tripId || '')}">
      <div class="form-grid">
        <label class="field"><span>Tipo</span><select name="type">
          ${['flight', 'hotel', 'car', 'other'].map((t) => `<option value="${t}" ${r.type === t ? 'selected' : ''}>${typeLabel(t)}</option>`).join('')}
        </select></label>
        <label class="field"><span>Estado</span><select name="status">
          <option value="confirmed" ${r.status !== 'canceled' ? 'selected' : ''}>Confirmada</option>
          <option value="canceled" ${r.status === 'canceled' ? 'selected' : ''}>Cancelada</option>
        </select></label>
        <label class="field full"><span>Título</span><input name="title" value="${esc(r.title || '')}" placeholder="Vuelo a Bariloche" required></label>
        <label class="field"><span>Proveedor</span><input name="provider" value="${esc(r.provider || '')}" placeholder="aerolineas"></label>
        <label class="field"><span>Localizador</span><input name="provider_locator" value="${esc(r.provider_locator || '')}" placeholder="ABC123"></label>
        <label class="field full"><span>Destino</span><input name="destination" value="${esc(r.destination || '')}" placeholder="Bariloche" required></label>
        <label class="field"><span>Inicio</span><input type="datetime-local" name="start_at" value="${esc(toLocalInput(r.start_at))}"></label>
        <label class="field"><span>Fin</span><input type="datetime-local" name="end_at" value="${esc(toLocalInput(r.end_at))}"></label>
        <label class="field full"><span>Detalles (JSON opcional)</span><textarea name="details" rows="4" placeholder='{"habitaciones":2}'>${esc(r.details ? JSON.stringify(r.details, null, 2) : '{}')}</textarea></label>
      </div>
      <div class="form-error hidden"></div>
      <div class="form-actions">
        <button class="btn btn-outline" type="button" data-modal-close>Cancelar</button>
        <button class="btn btn-primary" type="submit">${editing ? 'Guardar cambios' : 'Crear reserva'}</button>
      </div>
    </form>`,
  });
}

function openShareModal(tripId) {
  openModal({
    title: 'Compartir viaje',
    subtitle: 'La otra persona podrá verlo, pero no editarlo.',
    body: `<form data-form="share" class="stack">
      <input type="hidden" name="trip_id" value="${esc(tripId)}">
      <label class="field"><span>Email del invitado</span><input name="guest_email" type="email" placeholder="amigo@ejemplo.com" required></label>
      <div class="form-error hidden"></div>
      <div class="form-actions"><button class="btn btn-outline" type="button" data-modal-close>Cancelar</button><button class="btn btn-primary" type="submit">${icon('share', 15)} Compartir</button></div>
    </form>`,
  });
}

function openInquireModal(recId) {
  openModal({
    title: 'Consultar agencia',
    subtitle: 'La agencia va a recibir tu mensaje con el contexto de la recomendación.',
    body: `<form data-form="inquire-rec" class="stack">
      <input type="hidden" name="id" value="${esc(recId)}">
      <label class="field"><span>Mensaje</span><textarea name="message" rows="5" placeholder="Me interesa esta propuesta, ¿incluye traslados?" required>Me interesa esta propuesta. ¿Podés contarme más sobre disponibilidad y qué incluye?</textarea></label>
      <div class="form-error hidden"></div>
      <div class="form-actions"><button class="btn btn-outline" type="button" data-modal-close>Cancelar</button><button class="btn btn-coral" type="submit">${icon('send', 15)} Enviar consulta</button></div>
    </form>`,
  });
}

function openAgencyRequestModal(reservationId) {
  openModal({
    title: 'Consultar por una reserva',
    subtitle: 'Se envía a tu agencia favorita con los datos de la reserva.',
    body: `<form data-form="agency-request" class="stack">
      <input type="hidden" name="id" value="${esc(reservationId)}">
      <label class="field"><span>Mensaje</span><textarea name="message" rows="5" placeholder="¿Podés revisar esta reserva?" required>Hola, quisiera consultar por esta reserva y las opciones disponibles.</textarea></label>
      <div class="form-error hidden"></div>
      <div class="form-actions"><button class="btn btn-outline" type="button" data-modal-close>Cancelar</button><button class="btn btn-primary" type="submit">${icon('send', 15)} Enviar</button></div>
    </form>`,
  });
}

async function openChangesModal(reservationId) {
  const res = await api(`/reservations/${reservationId}/changes`);
  const changes = res.data || [];
  openModal({
    title: 'Historial de cambios',
    subtitle: `${changes.length} ${changes.length === 1 ? 'cambio registrado' : 'cambios registrados'}`,
    wide: true,
    body: changes.length ? `<div class="stack">${changes.map((c) => `<div class="account-card">
      <div class="row-between"><strong>${esc(sourceLabel(c.source))}</strong><span class="muted small">${esc(formatDateTime(c.created_at))}</span></div>
      <div class="row" style="margin-top:6px">${(c.changed_fields || []).map((f) => `<span class="pill">${esc(f)}</span>`).join('')}</div>
    </div>`).join('')}</div>` : '<div class="muted small">Esta reserva todavía no tiene cambios registrados.</div>',
  });
}

async function openSocialModal(tripId) {
  const networks = await api('/social/networks');
  state.networks = networks.data || {};
  const entries = Object.entries(state.networks);
  const initial = entries[0]?.[0] || 'x';
  const preview = await api(`/trips/${tripId}/social/preview`, { method: 'POST', body: { network: initial } });
  openModal({
    title: 'Publicar viaje en redes',
    subtitle: 'La vista previa no incluye localizadores ni datos personales.',
    body: `<div class="stack">
      <label class="field"><span>Red social</span><select id="social-network">${entries.map(([key, name]) => `<option value="${esc(key)}">${esc(name)}</option>`).join('')}</select></label>
      <div class="social-preview" id="social-preview">${esc(preview.text)}</div>
      <div class="hint">${icon('shield', 14)} ${preview.reservations} reservas confirmadas · sin PII</div>
      <div class="form-actions between">
        <span class="muted tiny" id="social-result"></span>
        <div class="row">
          <button class="btn btn-outline" type="button" data-action="social-refresh" data-id="${esc(tripId)}">${icon('refresh', 15)} Actualizar</button>
          <button class="btn btn-coral" type="button" data-action="social-publish" data-id="${esc(tripId)}">${icon('send', 15)} Publicar</button>
        </div>
      </div>
    </div>`,
  });
}

async function handleLocation(tripId) {
  if (!navigator.geolocation) {
    toast('Tu navegador no soporta geolocalización.', 'error');
    return;
  }
  toast('Pidiendo permiso de ubicación...', 'info');
  const position = await new Promise((resolve, reject) => {
    navigator.geolocation.getCurrentPosition(resolve, reject, { timeout: 9000, maximumAge: 60000 });
  }).catch(() => null);
  if (!position) {
    toast('No pudimos obtener tu ubicación. Podés habilitar el permiso del navegador.', 'error');
    return;
  }
  const { latitude, longitude } = position.coords;
  const out = await api(`/trips/${tripId}/context-location?consent=true&lat=${latitude}&lng=${longitude}`, { method: 'POST' });
  if (!out.available) {
    openModal({
      title: 'Contexto de ubicación',
      body: `<div class="empty"><div class="empty-icon">${icon('location', 24)}</div><strong>No disponible</strong><p>${esc(locationReason(out.reason))}</p></div>`,
    });
    return;
  }
  openModal({
    title: 'Contexto de ubicación',
    subtitle: `Destino: ${out.destination}`,
    body: `<div class="stack">
      <div class="stat"><div class="stat-top"><span class="stat-icon gold">${icon('location', 18)}</span></div><div class="stat-value">${Math.round(out.distance_km)} km</div><div class="stat-label">Distancia aproximada al destino</div></div>
      <a class="btn btn-primary" href="${esc(out.navigation_url)}" target="_blank" rel="noopener">${icon('external', 16)} Abrir navegación</a>
      <p class="hint">Tu ubicación se usa solo para calcular la distancia. No se guarda en el itinerario.</p>
    </div>`,
  });
}

/* Actions and forms */

async function onClick(event) {
  const target = event.target.closest('[data-action]');
  if (!target) return;
  const action = target.dataset.action;
  if (target.tagName === 'A') event.preventDefault();
  try {
    await handleAction(action, target);
  } catch (err) {
    toast(friendlyError(err), 'error');
  }
}

async function handleAction(action, el) {
  switch (action) {
    case 'auth-mode':
      state.authMode = el.dataset.mode === 'register' ? 'register' : 'login';
      showAuth();
      break;
    case 'fill-demo': {
      state.authMode = 'login';
      showAuth();
      const form = document.querySelector('form[data-form="auth"]');
      if (form) {
        form.elements.email.value = 'analyst@trotamundos.dev';
        form.elements.password.value = 'analyst12345';
      }
      break;
    }
    case 'logout':
      await logout();
      state.user = null;
      showAuth();
      break;
    case 'retry-route':
    case 'refresh-view':
      await renderRoute();
      break;
    case 'new-reservation':
      openReservationModal(null, el.dataset.trip || '');
      break;
    case 'edit-reservation': {
      let res = state.currentTrip?.reservations?.find((r) => r.id === el.dataset.id);
      if (!res) res = await api(`/reservations/${el.dataset.id}`);
      openReservationModal(res, res.trip_id || '');
      break;
    }
    case 'sync-reservation': {
      const res = await api(`/reservations/${el.dataset.id}/sync`, { method: 'POST' });
      toast(res.stale ? 'El proveedor no respondió. La reserva quedó marcada como desactualizada.' : 'Reserva sincronizada con el proveedor.', res.stale ? 'info' : 'success');
      await renderRoute();
      break;
    }
    case 'delete-reservation': {
      const ok = await confirmDialog({ title: 'Eliminar reserva', message: '¿Querés eliminar esta reserva? Esta acción no se puede deshacer.', confirmLabel: 'Eliminar', danger: true });
      if (!ok) return;
      await api(`/reservations/${el.dataset.id}?confirm=true`, { method: 'DELETE' });
      toast('Reserva eliminada.');
      await renderRoute();
      break;
    }
    case 'changes-reservation':
      await openChangesModal(el.dataset.id);
      break;
    case 'contact-reservation':
      openAgencyRequestModal(el.dataset.id);
      break;
    case 'share-trip':
      openShareModal(el.dataset.id);
      break;
    case 'revoke-share': {
      const ok = await confirmDialog({ title: 'Revocar acceso', message: 'La persona ya no va a poder ver este viaje compartido.', confirmLabel: 'Revocar', danger: true });
      if (!ok) return;
      await api(`/shares/${el.dataset.id}`, { method: 'DELETE' });
      toast('Acceso revocado.');
      await renderRoute();
      break;
    }
    case 'social-trip':
      await openSocialModal(el.dataset.id);
      break;
    case 'location-trip':
      await handleLocation(el.dataset.id);
      break;
    case 'delete-trip': {
      const ok = await confirmDialog({ title: 'Eliminar viaje', message: 'Solo se pueden eliminar viajes archivados. ¿Querés continuar?', confirmLabel: 'Eliminar viaje', danger: true });
      if (!ok) return;
      await api(`/trips/${el.dataset.id}?confirm=true`, { method: 'DELETE' });
      toast('Viaje eliminado.');
      location.hash = '#/trips';
      break;
    }
    case 'trip-tab':
      state.trips.tab = el.dataset.tab === 'archived' ? 'archived' : 'active';
      await renderTrips();
      break;
    case 'scan-mailbox': {
      const out = await api(`/mail/accounts/${el.dataset.id}/scan`, { method: 'POST' });
      toast(`${out.events_created} eventos encolados. El worker los procesa en segundos.`, 'info');
      setTimeout(() => { if (state.route === 'mail') renderRoute(); }, 2400);
      break;
    }
    case 'revoke-mailbox': {
      const ok = await confirmDialog({ title: 'Revocar cuenta', message: 'Se va a dejar de escanear esta casilla. ¿Querés revocarla?', confirmLabel: 'Revocar', danger: true });
      if (!ok) return;
      await api(`/mail/accounts/${el.dataset.id}`, { method: 'DELETE' });
      toast('Cuenta revocada.');
      await renderRoute();
      break;
    }
    case 'delete-whitelist':
      await api(`/mail/whitelist/${el.dataset.id}`, { method: 'DELETE' });
      toast('Regla eliminada.');
      await renderRoute();
      break;
    case 'delete-filter':
      await api(`/mail/filters/${el.dataset.id}`, { method: 'DELETE' });
      toast('Filtro eliminado.');
      await renderRoute();
      break;
    case 'toggle-filter': {
      const current = state.mail.filters.find((f) => f.id === el.dataset.id);
      if (!current) return;
      await api(`/mail/filters/${current.id}`, {
        method: 'PATCH',
        body: { field: current.field, operator: current.operator, value: current.value, active: !current.active },
      });
      toast(current.active ? 'Filtro desactivado.' : 'Filtro activado.');
      await renderRoute();
      break;
    }
    case 'set-favorite':
      await api('/agencies/favorite', { method: 'PUT', body: { agency_id: el.dataset.id } });
      toast('Agencia favorita actualizada.');
      await renderRoute();
      break;
    case 'remove-favorite': {
      const ok = await confirmDialog({ title: 'Quitar agencia favorita', message: 'Dejás de recibir recomendaciones de esta agencia.', confirmLabel: 'Quitar', danger: true });
      if (!ok) return;
      await api('/agencies/favorite?confirm=true', { method: 'DELETE' });
      toast('Agencia favorita eliminada.');
      await renderRoute();
      break;
    }
    case 'discard-rec': {
      const ok = await confirmDialog({ title: 'Descartar recomendación', message: 'No vas a volver a ver esta propuesta. ¿Querés descartarla?', confirmLabel: 'Descartar', danger: true });
      if (!ok) return;
      await api(`/recommendations/${el.dataset.id}/discard`, { method: 'POST' });
      toast('Recomendación descartada.');
      await renderRoute();
      break;
    }
    case 'inquire-rec':
      openInquireModal(el.dataset.id);
      break;
    case 'close-request':
      await api(`/agency-requests/${el.dataset.id}/close`, { method: 'POST' });
      toast('Consulta cerrada.');
      await renderRoute();
      break;
    case 'mark-read':
      await api(`/notifications/${el.dataset.id}/read`, { method: 'POST' });
      await loadNotifications();
      await renderRoute();
      break;
    case 'mark-all-read': {
      const unread = state.notifications.filter((n) => !n.read_at);
      await Promise.all(unread.map((n) => api(`/notifications/${n.id}/read`, { method: 'POST' })));
      toast('Notificaciones marcadas como leídas.');
      await loadNotifications();
      await renderRoute();
      break;
    }
    case 'social-refresh':
      await refreshSocialPreview(el.dataset.id);
      break;
    case 'social-publish': {
      const network = document.getElementById('social-network')?.value || 'x';
      const post = await api(`/trips/${el.dataset.id}/social/share`, { method: 'POST', body: { network } });
      const result = document.getElementById('social-result');
      if (result) result.innerHTML = `Publicado en ${esc(networkName(network))}: <a href="${esc(post.url)}" target="_blank" rel="noopener">ver publicación</a>`;
      toast('Publicación simulada creada.');
      break;
    }
    default:
      break;
  }
}

async function refreshSocialPreview(tripId) {
  const network = document.getElementById('social-network')?.value || 'x';
  const preview = await api(`/trips/${tripId}/social/preview`, { method: 'POST', body: { network } });
  const box = document.getElementById('social-preview');
  if (box) box.textContent = preview.text;
  const hint = document.querySelector('#social-result');
  if (hint) hint.textContent = `${preview.network_name} · ${preview.reservations} reservas confirmadas`;
}

async function onSubmit(event) {
  const form = event.target.closest('form[data-form]');
  if (!form) return;
  event.preventDefault();
  const type = form.dataset.form;
  try {
    if (type === 'auth') return await submitAuth(form);
    if (type === 'reservation') return await submitReservation(form);
    if (type === 'share') return await submitShare(form);
    if (type === 'mail-account') return await submitMailAccount(form);
    if (type === 'mock-mail') return await submitMockMail(form);
    if (type === 'whitelist') return await submitWhitelist(form);
    if (type === 'filter') return await submitFilter(form);
    if (type === 'inquire-rec') return await submitInquire(form);
    if (type === 'agency-request') return await submitAgencyRequest(form);
    if (type === 'analytics-range') return await submitAnalyticsRange(form);
  } catch (err) {
    showFormError(form, friendlyError(err));
  }
}

async function submitAuth(form) {
  clearFormError(form);
  setFormBusy(form, true, 'Ingresando...');
  try {
    const mode = form.dataset.mode;
    const data = Object.fromEntries(new FormData(form));
    const payload = {
      email: String(data.email || '').trim(),
      password: String(data.password || ''),
    };
    if (mode === 'register') payload.full_name = String(data.full_name || '').trim();
    const out = await api(mode === 'register' ? '/auth/register' : '/auth/login', {
      method: 'POST', body: payload, auth: false, retry: false,
    });
    session.set(out);
    state.user = out.user;
    state.authMode = 'login';
    if (!location.hash) history.replaceState(null, '', '#/dashboard');
    startApp();
    toast(mode === 'register' ? 'Cuenta creada. ¡Bienvenido a Trotamundos!' : 'Sesión iniciada.');
  } catch (err) {
    showFormError(form, friendlyError(err));
  } finally {
    setFormBusy(form, false);
  }
}

async function submitReservation(form) {
  clearFormError(form);
  setFormBusy(form, true, 'Guardando...');
  try {
    const raw = Object.fromEntries(new FormData(form));
    const body = {
      type: raw.type,
      title: String(raw.title || '').trim(),
      provider: String(raw.provider || '').trim(),
      provider_locator: String(raw.provider_locator || '').trim(),
      destination: String(raw.destination || '').trim(),
      status: raw.status || 'confirmed',
      details: raw.details ? parseJsonObject(raw.details) : {},
    };
    if (raw.start_at) body.start_at = fromLocalInput(raw.start_at);
    if (raw.end_at) body.end_at = fromLocalInput(raw.end_at);
    if (raw.trip_id) body.trip_id = raw.trip_id;
    if (raw.id) {
      await api(`/reservations/${raw.id}`, { method: 'PATCH', body });
      toast('Reserva actualizada.');
    } else {
      await api('/reservations', { method: 'POST', body });
      toast('Reserva creada y agrupada en tu itinerario.');
    }
    closeModal();
    await renderRoute();
  } finally {
    setFormBusy(form, false);
  }
}

async function submitShare(form) {
  clearFormError(form);
  setFormBusy(form, true, 'Compartiendo...');
  try {
    const raw = Object.fromEntries(new FormData(form));
    await api(`/trips/${raw.trip_id}/shares`, { method: 'POST', body: { guest_email: String(raw.guest_email || '').trim() } });
    toast('Viaje compartido. El invitado lo verá al ingresar con ese email.');
    closeModal();
    await renderRoute();
  } finally {
    setFormBusy(form, false);
  }
}

async function submitMailAccount(form) {
  clearFormError(form);
  setFormBusy(form, true, 'Vinculando...');
  try {
    const raw = Object.fromEntries(new FormData(form));
    await api('/mail/accounts', {
      method: 'POST',
      body: {
        provider: raw.provider,
        email: String(raw.email || '').trim(),
        access_token: String(raw.access_token || 'demo-token'),
        scopes: ['mail.read'],
      },
    });
    toast('Cuenta vinculada.');
    await renderRoute();
  } finally {
    setFormBusy(form, false);
  }
}

async function submitMockMail(form) {
  clearFormError(form);
  setFormBusy(form, true, 'Procesando...');
  try {
    const raw = Object.fromEntries(new FormData(form));
    const email = String(raw.email || '').trim();
    const mockKey = String(raw.mock_key || '').trim();
    if (!mockKey) throw new Error('La clave mock es obligatoria');
    state.mockKey = mockKey;
    localStorage.setItem('trotamundos.mockKey', mockKey);
    await api(`/mock/mail/${encodeURIComponent(email)}/messages`, {
      method: 'POST',
      headers: { 'X-Mock-Key': mockKey },
      body: {
        from: String(raw.from || '').trim(),
        subject: String(raw.subject || '').trim(),
        body: String(raw.body || ''),
      },
    });
    let created = 0;
    if (raw.scan_after) {
      const acc = state.mail.accounts.find((a) => a.email.toLowerCase() === email.toLowerCase() && a.status === 'active');
      if (acc) {
        const out = await api(`/mail/accounts/${acc.id}/scan`, { method: 'POST' });
        created = out.events_created || 0;
      }
    }
    toast(created ? `Email agregado. ${created} eventos encolados; se procesan en segundos.` : 'Email agregado a la bandeja mock. Vinculá esa cuenta para escanearla.', 'info');
    if (created) setTimeout(() => { if (state.route === 'mail') renderRoute(); }, 2400);
  } finally {
    setFormBusy(form, false);
  }
}

async function submitWhitelist(form) {
  clearFormError(form);
  setFormBusy(form, true, 'Agregando...');
  try {
    const raw = Object.fromEntries(new FormData(form));
    await api('/mail/whitelist', { method: 'POST', body: { pattern: String(raw.pattern || '').trim() } });
    toast('Regla de whitelist agregada.');
    await renderRoute();
  } finally {
    setFormBusy(form, false);
  }
}

async function submitFilter(form) {
  clearFormError(form);
  setFormBusy(form, true, 'Agregando...');
  try {
    const raw = Object.fromEntries(new FormData(form));
    await api('/mail/filters', {
      method: 'POST',
      body: {
        field: raw.field,
        operator: raw.operator,
        value: String(raw.value || '').trim(),
        active: form.elements.active.checked,
      },
    });
    toast('Filtro agregado.');
    await renderRoute();
  } finally {
    setFormBusy(form, false);
  }
}

async function submitInquire(form) {
  clearFormError(form);
  setFormBusy(form, true, 'Enviando...');
  try {
    const raw = Object.fromEntries(new FormData(form));
    await api(`/recommendations/${raw.id}/inquire`, { method: 'POST', body: { message: String(raw.message || '').trim() } });
    toast('Consulta enviada a tu agencia.');
    closeModal();
    await renderRoute();
  } finally {
    setFormBusy(form, false);
  }
}

async function submitAgencyRequest(form) {
  clearFormError(form);
  setFormBusy(form, true, 'Enviando...');
  try {
    const raw = Object.fromEntries(new FormData(form));
    await api(`/reservations/${raw.id}/agency-request`, { method: 'POST', body: { message: String(raw.message || '').trim() } });
    toast('Consulta enviada a tu agencia.');
    closeModal();
    await renderRoute();
  } finally {
    setFormBusy(form, false);
  }
}

async function submitAnalyticsRange(form) {
  clearFormError(form);
  setFormBusy(form, true, 'Aplicando...');
  try {
    const raw = Object.fromEntries(new FormData(form));
    if (!raw.from || !raw.to) throw new Error('Elegí un rango de fechas');
    if (raw.from > raw.to) throw new Error('La fecha desde debe ser anterior a la fecha hasta');
    state.analyticsRange = { from: raw.from, to: raw.to };
    await renderRoute();
  } finally {
    setFormBusy(form, false);
  }
}

/* Inputs and helpers */

document.addEventListener('input', (event) => {
  if (event.target.id === 'trip-search') {
    state.trips.query = event.target.value;
    applyTripSearch();
  }
});
document.addEventListener('click', onClick);
document.addEventListener('submit', onSubmit);
window.addEventListener('hashchange', () => {
  if (state.user) renderRoute();
  else showAuth();
});

async function loadNotifications() {
  const res = await api('/notifications');
  state.notifications = res.data || [];
  updateNotifBadge();
}

function updateNotifBadge() {
  const unread = state.notifications.filter((n) => !n.read_at).length;
  const navBadge = document.getElementById('nav-notif-badge');
  const bell = document.getElementById('bell-count');
  if (navBadge) {
    navBadge.textContent = String(unread);
    navBadge.classList.toggle('hidden', unread === 0);
  }
  if (bell) {
    bell.textContent = String(unread);
    bell.classList.toggle('hidden', unread === 0);
  }
}

function closeModal() {
  const btn = document.querySelector('[data-modal-close]');
  if (btn) btn.click();
}

function setFormBusy(form, busy, label) {
  const button = form.querySelector('button[type=submit]');
  if (!button) return;
  if (busy) {
    button.dataset.label = button.innerHTML;
    button.disabled = true;
    button.innerHTML = `<span class="spinner" style="width:15px;height:15px;border-color:rgba(255,255,255,.35);border-top-color:currentColor"></span> ${esc(label || 'Guardando...')}`;
  } else {
    button.disabled = false;
    if (button.dataset.label) button.innerHTML = button.dataset.label;
  }
}

function showFormError(form, message) {
  let box = form.querySelector('.form-error');
  if (!box) {
    box = document.createElement('div');
    box.className = 'form-error';
    form.prepend(box);
  }
  box.textContent = message;
  box.classList.remove('hidden');
}

function clearFormError(form) {
  const box = form.querySelector('.form-error');
  if (box) {
    box.textContent = '';
    box.classList.add('hidden');
  }
}

function friendlyError(err) {
  const message = err?.message || 'Ocurrió un error inesperado';
  if (message === 'Failed to fetch' || message.includes('NetworkError')) {
    return 'No pudimos conectar con el servidor. Verificá que la API esté corriendo.';
  }
  return message;
}

function parseJsonObject(value) {
  const parsed = JSON.parse(String(value || '').trim() || '{}');
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('Los detalles deben ser un objeto JSON');
  }
  return parsed;
}

function firstName(name = '') {
  return String(name).trim().split(/\s+/)[0] || 'viajero';
}

function defaultRange() {
  const to = new Date();
  const from = new Date();
  from.setMonth(from.getMonth() - 3);
  return { from: isoDay(from), to: isoDay(to) };
}

function isoDay(date) {
  return date.toISOString().slice(0, 10);
}

function locationReason(reason) {
  const reasons = {
    location_consent_required: 'Necesitamos tu consentimiento y coordenadas para calcular la distancia.',
    destination_without_coordinates: 'Todavía no tenemos coordenadas para este destino.',
  };
  return reasons[reason] || 'No pudimos calcular el contexto de ubicación.';
}
