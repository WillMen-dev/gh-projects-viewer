// Cliente HTTP mínimo: envía cookies de sesión y el token CSRF en peticiones que modifican datos.
(function () {
  'use strict';
  let csrfToken = '';

  class ApiError extends Error {
    constructor(status, code, message) {
      super(message || 'Error');
      this.status = status;
      this.code = code;
    }
  }

  async function request(method, path, body) {
    const headers = { 'Accept': 'application/json' };
    const opts = { method, headers, credentials: 'same-origin', cache: 'no-store' };
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    if (method !== 'GET' && csrfToken) headers['X-CSRF-Token'] = csrfToken;

    let res;
    try {
      res = await fetch(path, opts);
    } catch (e) {
      throw new ApiError(0, 'network', 'No hay conexión con el servidor');
    }
    let data = null;
    try { data = await res.json(); } catch (e) { /* respuesta sin JSON */ }
    if (!res.ok) {
      throw new ApiError(res.status, data && data.error, (data && data.message) || 'Error ' + res.status);
    }
    return data;
  }

  // h(): crea elementos de forma segura. El texto SIEMPRE se inserta con textContent
  // (nunca innerHTML), así los datos que vienen de GitHub no pueden inyectar HTML/JS (XSS).
  function h(tag, attrs, ...children) {
    const el = document.createElement(tag);
    if (attrs) {
      for (const [k, v] of Object.entries(attrs)) {
        if (v === undefined || v === null || v === false) continue;
        if (k === 'class') el.className = v;
        else if (k === 'text') el.textContent = v;
        else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2), v);
        else if (k === 'href') el.setAttribute('href', safeUrl(v));
        else if (k === 'src') el.setAttribute('src', safeUrl(v));
        else el.setAttribute(k, v === true ? '' : String(v));
      }
    }
    for (const c of children.flat()) {
      if (c === null || c === undefined || c === false) continue;
      el.append(c instanceof Node ? c : document.createTextNode(String(c)));
    }
    return el;
  }

  // Solo se permiten enlaces http(s) o rutas internas (bloquea "javascript:").
  function safeUrl(u) {
    const s = String(u || '');
    if (s.startsWith('#') || s.startsWith('/')) return s;
    try {
      const url = new URL(s);
      if (url.protocol === 'https:' || url.protocol === 'http:') return url.href;
    } catch (e) { /* inválida */ }
    return '#';
  }

  window.API = {
    ApiError,
    setCsrf(t) { csrfToken = t || ''; },
    get: (p) => request('GET', p),
    post: (p, b) => request('POST', p, b === undefined ? {} : b),
    patch: (p, b) => request('PATCH', p, b),
    h,
    safeUrl,
  };
})();
