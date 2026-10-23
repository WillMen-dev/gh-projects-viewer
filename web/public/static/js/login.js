(function () {
  'use strict';
  const form = document.getElementById('login-form');
  const errorEl = document.getElementById('login-error');

  // Si ya hay sesión válida, ir directo a la aplicación.
  API.get('/api/auth/me').then(() => { window.location.replace('/'); }).catch(() => {});

  form.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    errorEl.hidden = true;
    const btn = form.querySelector('button');
    const email = form.email.value.trim();
    const password = form.password.value;
    if (!email || !password) {
      errorEl.textContent = 'Ingrese correo y contraseña.';
      errorEl.hidden = false;
      return;
    }
    btn.disabled = true;
    btn.textContent = 'Verificando…';
    try {
      await API.post('/api/auth/login', { email, password });
      form.password.value = '';
      window.location.replace('/');
    } catch (e) {
      errorEl.textContent = e.message;
      errorEl.hidden = false;
      form.password.value = '';
      form.password.focus();
    } finally {
      btn.disabled = false;
      btn.textContent = 'Ingresar';
    }
  });
})();
