const ACCESS_KEY = 'trotamundos.access';
const REFRESH_KEY = 'trotamundos.refresh';
const USER_KEY = 'trotamundos.user';

export class ApiError extends Error {
  constructor(status, code, message) {
    super(message || 'No se pudo completar la operación');
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

export const session = {
  get access() {
    return localStorage.getItem(ACCESS_KEY) || '';
  },
  get refresh() {
    return localStorage.getItem(REFRESH_KEY) || '';
  },
  get user() {
    try {
      return JSON.parse(localStorage.getItem(USER_KEY) || 'null');
    } catch {
      return null;
    }
  },
  set(data) {
    if (data?.tokens) {
      if (data.tokens.access_token) localStorage.setItem(ACCESS_KEY, data.tokens.access_token);
      if (data.tokens.refresh_token) localStorage.setItem(REFRESH_KEY, data.tokens.refresh_token);
    }
    if (data?.user) localStorage.setItem(USER_KEY, JSON.stringify(data.user));
  },
  setUser(user) {
    if (user) localStorage.setItem(USER_KEY, JSON.stringify(user));
  },
  clear() {
    localStorage.removeItem(ACCESS_KEY);
    localStorage.removeItem(REFRESH_KEY);
    localStorage.removeItem(USER_KEY);
  },
};

let refreshRequest = null;

async function refreshTokens() {
  if (!session.refresh) return false;
  if (!refreshRequest) {
    refreshRequest = fetch('/auth/refresh', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: session.refresh }),
    })
      .then(async (res) => {
        if (!res.ok) return false;
        const data = await res.json();
        session.set(data);
        return true;
      })
      .catch(() => false)
      .finally(() => {
        refreshRequest = null;
      });
  }
  return refreshRequest;
}

export async function api(path, options = {}) {
  const { method = 'GET', body, headers = {}, auth = true, retry = true } = options;
  const requestHeaders = { ...headers };
  if (body !== undefined) requestHeaders['Content-Type'] = 'application/json';
  if (auth && session.access) requestHeaders.Authorization = `Bearer ${session.access}`;

  const res = await fetch(path, {
    method,
    headers: requestHeaders,
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  if (res.status === 401 && auth && retry && session.refresh) {
    const refreshed = await refreshTokens();
    if (refreshed) {
      return api(path, { ...options, retry: false });
    }
    session.clear();
  }

  if (res.status === 204) return null;

  const text = await res.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }

  if (!res.ok) {
    const message = data?.error?.message || `Error HTTP ${res.status}`;
    throw new ApiError(res.status, data?.error?.code || 'http_error', message);
  }
  return data;
}

export async function logout() {
  const refresh = session.refresh;
  try {
    await api('/auth/logout', {
      method: 'POST',
      body: { refresh_token: refresh },
      auth: false,
      retry: false,
    });
  } catch {
    // La sesión local se limpia igualmente.
  }
  session.clear();
}
