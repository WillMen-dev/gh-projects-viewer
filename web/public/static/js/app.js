(function () {
  'use strict';
  const { h } = API;
  const app = document.getElementById('app');
  let me = null;

  // ---------- utilidades ----------
  const fmtDate = (s) => {
    if (!s) return '—';
    const [y, m, d] = s.slice(0, 10).split('-');
    return `${d}/${m}/${y}`;
  };
  const fmtDateTime = (s) => {
    if (!s) return '—';
    const d = new Date(s);
    return isNaN(d) ? '—' : d.toLocaleString('es-CO', { dateStyle: 'short', timeStyle: 'short' });
  };
  const todayISO = () => {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  };
  const typeLabel = { ISSUE: 'Issue', PULL_REQUEST: 'Pull request', DRAFT_ISSUE: 'Borrador', REDACTED: 'Restringido' };
  const slug = (s) => String(s || '').toLowerCase().replace(/[^a-z0-9]+/g, '-');

  function setView(...nodes) {
    app.replaceChildren(...nodes.filter((n) => n !== null && n !== undefined && n !== false));
    window.scrollTo(0, 0);
  }

  function errorBox(e, retry) {
    return h('div', { class: 'alert alert-error', role: 'alert' },
      h('strong', { text: 'No se pudo cargar la información. ' }), e.message || String(e),
      retry ? h('button', { class: 'btn btn-ghost', type: 'button', onclick: retry, text: 'Reintentar' }) : null);
  }

  function handleAuthError(e) {
    if (e.status === 401) { window.location.replace('/login'); return true; }
    if (e.code === 'password_change_required') { location.hash = '#/password'; return true; }
    return false;
  }

  function markNav(name) {
    document.querySelectorAll('[data-nav]').forEach((a) => a.classList.toggle('active', a.dataset.nav === name));
  }

  // ---------- modal ----------
  const modal = document.getElementById('modal');
  const modalBody = document.getElementById('modal-body');
  let lastFocus = null;
  function openModal(content) {
    lastFocus = document.activeElement;
    modalBody.replaceChildren(content);
    modal.hidden = false;
    modal.querySelector('.modal-close').focus();
  }
  function closeModal() {
    modal.hidden = true;
    modalBody.replaceChildren();
    if (lastFocus) lastFocus.focus();
  }
  modal.addEventListener('click', (e) => { if (e.target.closest('[data-close]')) closeModal(); });
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape' && !modal.hidden) closeModal(); });

  // ---------- vista: lista de proyectos ----------
  async function viewProjects() {
    markNav('projects');
    setView(h('p', { class: 'muted', text: 'Cargando proyectos…' }));
    let data;
    try {
      data = await API.get('/api/projects');
    } catch (e) {
      if (handleAuthError(e)) return;
      setView(errorBox(e, viewProjects));
      return;
    }
    const all = data.projects;
    const search = h('input', { type: 'search', placeholder: 'Buscar proyecto…', 'aria-label': 'Buscar proyecto', class: 'input' });
    const showClosed = h('input', { type: 'checkbox', id: 'show-closed' });
    const grid = h('div', { class: 'project-grid' });

    function render() {
      const q = search.value.trim().toLowerCase();
      const list = all.filter((p) => (showClosed.checked || !p.closed) &&
        (!q || p.title.toLowerCase().includes(q) || (p.shortDescription || '').toLowerCase().includes(q)));
      grid.replaceChildren(...(list.length ? list.map((p) =>
        h('a', { class: 'project-card' + (p.closed ? ' closed' : ''), href: '#/project/' + encodeURIComponent(p.id) },
          h('div', { class: 'project-card-head' },
            h('span', { class: 'project-number', text: '#' + p.number }),
            p.closed ? h('span', { class: 'badge', text: 'Cerrado' }) : null),
          h('h3', { text: p.title }),
          h('p', { class: 'muted', text: p.shortDescription || 'Sin descripción' }),
          h('span', { class: 'small muted', text: 'Actualizado: ' + fmtDateTime(p.updatedAt) }))
      ) : [h('p', { class: 'muted', text: 'No hay proyectos que coincidan.' })]));
    }
    search.addEventListener('input', render);
    showClosed.addEventListener('change', render);
    render();

    setView(
      h('div', { class: 'page-head' },
        h('div', {}, h('h1', { text: 'Proyectos' }),
          h('p', { class: 'muted', text: `${all.length} proyecto(s) disponibles en GitHub Projects.` }))),
      h('div', { class: 'toolbar' }, search,
        h('label', { class: 'check' }, showClosed, ' Mostrar cerrados')),
      grid);
  }

  // ---------- vista: detalle de proyecto ----------
  async function viewProject(id) {
    markNav('projects');
    setView(h('p', { class: 'muted', text: 'Consultando GitHub…' }));
    let data;
    try {
      data = await API.get('/api/projects/' + encodeURIComponent(id));
    } catch (e) {
      if (handleAuthError(e)) return;
      setView(h('a', { href: '#/', class: 'back', text: '← Proyectos' }), errorBox(e, () => viewProject(id)));
      return;
    }
    const { project, tasks, summary } = data;
    const doneSet = new Set((data.doneStatuses || []).map((s) => s.toLowerCase()));
    const isDone = (t) => doneSet.has((t.status || '').toLowerCase()) || (!t.status && (t.state === 'CLOSED' || t.state === 'MERGED'));
    const today = todayISO();
    const isOverdue = (t) => t.dueDate && t.dueDate < today && !isDone(t);

    // Opciones de filtros
    const statuses = data.statusOptions.length ? [...data.statusOptions] : [...new Set(tasks.map((t) => t.status).filter(Boolean))];
    if (tasks.some((t) => !t.status)) statuses.push('');
    const priorities = data.priorityOptions.length ? data.priorityOptions : [...new Set(tasks.map((t) => t.priority).filter(Boolean))].sort();
    const assignees = [...new Set(tasks.flatMap((t) => t.assignees.map((a) => a.login)))].sort();

    const sel = (label, opts, extra) => h('select', { class: 'input', 'aria-label': label },
      h('option', { value: '*', text: label }),
      extra || null,
      ...opts.map((o) => h('option', { value: o, text: o || 'Sin valor' })));

    const fSearch = h('input', { type: 'search', class: 'input grow', placeholder: 'Buscar por título, número, repositorio o etiqueta…', 'aria-label': 'Buscar tarea' });
    const fStatus = sel('Todos los estados', statuses);
    const fPrio = sel('Todas las prioridades', priorities, h('option', { value: '', text: 'Sin prioridad' }));
    const fAssignee = sel('Todos los responsables', assignees, h('option', { value: '', text: 'Sin responsable' }));
    const fOverdue = h('input', { type: 'checkbox' });
    const counter = h('span', { class: 'muted small' });
    const content = h('div');
    let mode = 'board';
    let sortKey = 'dueDate', sortDir = 1;

    function filtered() {
      const q = fSearch.value.trim().toLowerCase();
      return tasks.filter((t) => {
        if (fStatus.value !== '*' && (t.status || '') !== fStatus.value) return false;
        if (fPrio.value !== '*' && (t.priority || '') !== fPrio.value) return false;
        if (fAssignee.value !== '*') {
          if (fAssignee.value === '' ? t.assignees.length > 0 : !t.assignees.some((a) => a.login === fAssignee.value)) return false;
        }
        if (fOverdue.checked && !isOverdue(t)) return false;
        if (q) {
          const hay = [t.title, t.number ? '#' + t.number : '', t.repository, ...t.labels, ...t.assignees.map((a) => a.login + ' ' + (a.name || ''))].join(' ').toLowerCase();
          if (!hay.includes(q)) return false;
        }
        return true;
      });
    }

    const assigneeChips = (t) => t.assignees.length
      ? h('div', { class: 'chips' }, ...t.assignees.map((a) => h('span', { class: 'chip', title: a.name || a.login }, a.avatarUrl ? h('img', { src: a.avatarUrl, alt: '', width: 16, height: 16, loading: 'lazy', referrerpolicy: 'no-referrer' }) : null, a.name || a.login)))
      : h('span', { class: 'muted small', text: 'Sin responsable' });

    const prioBadge = (p) => p ? h('span', { class: 'badge prio prio-' + slug(p), text: p }) : null;

    function dueLabel(t) {
      if (!t.dueDate) return null;
      const cls = isOverdue(t) ? 'due overdue' : 'due';
      return h('span', { class: cls, text: (isOverdue(t) ? 'Vencida · ' : 'Entrega ') + fmtDate(t.dueDate) });
    }

    function card(t) {
      return h('button', { class: 'task-card' + (isOverdue(t) ? ' is-overdue' : ''), type: 'button', onclick: () => openModal(taskDetail(t)) },
        h('div', { class: 'task-card-top' },
          h('span', { class: 'small muted', text: t.number ? `${t.repository.split('/').pop()} #${t.number}` : typeLabel[t.type] || t.type }),
          prioBadge(t.priority)),
        h('span', { class: 'task-title', text: t.title }),
        h('div', { class: 'task-card-bottom' }, assigneeChips(t), dueLabel(t)));
    }

    function renderBoard(list) {
      const cols = statuses.map((s) => ({ name: s, items: list.filter((t) => (t.status || '') === s) }));
      return h('div', { class: 'board' }, ...cols.map((c) =>
        h('section', { class: 'column' },
          h('header', { class: 'column-head' }, h('span', { text: c.name || 'Sin estado' }), h('span', { class: 'count', text: c.items.length })),
          h('div', { class: 'column-body' }, ...(c.items.length ? c.items.map(card) : [h('p', { class: 'muted small empty', text: 'Sin tareas' })])))));
    }

    const columns = [
      ['title', 'Tarea'], ['status', 'Estado'], ['priority', 'Prioridad'], ['assignees', 'Responsables'],
      ['estimate', 'Estimación'], ['startDate', 'Inicio'], ['dueDate', 'Entrega'],
    ];
    const sortVal = (t, k) => k === 'assignees' ? t.assignees.map((a) => a.login).join(',') : (t[k] || '');

    function renderTable(list) {
      const sorted = [...list].sort((a, b) => {
        const va = sortVal(a, sortKey), vb = sortVal(b, sortKey);
        if (!va && vb) return 1;
        if (va && !vb) return -1;
        const na = parseFloat(va), nb = parseFloat(vb);
        if (sortKey === 'estimate' && !isNaN(na) && !isNaN(nb)) return (na - nb) * sortDir;
        return String(va).localeCompare(String(vb), 'es') * sortDir;
      });
      return h('div', { class: 'table-wrap' }, h('table', { class: 'table' },
        h('thead', {}, h('tr', {}, ...columns.map(([k, label]) =>
          h('th', { scope: 'col' }, h('button', {
            class: 'th-btn', type: 'button', 'aria-sort': sortKey === k ? (sortDir > 0 ? 'ascending' : 'descending') : 'none',
            onclick: () => { if (sortKey === k) sortDir = -sortDir; else { sortKey = k; sortDir = 1; } render(); },
          }, label, sortKey === k ? (sortDir > 0 ? ' ▲' : ' ▼') : ''))))),
        h('tbody', {}, ...sorted.map((t) => h('tr', { class: isOverdue(t) ? 'row-overdue' : '' },
          h('td', {}, h('button', { class: 'link-btn', type: 'button', onclick: () => openModal(taskDetail(t)), text: t.title }),
            h('div', { class: 'small muted', text: t.number ? `${t.repository} #${t.number}` : typeLabel[t.type] || '' })),
          h('td', { text: t.status || '—' }),
          h('td', {}, prioBadge(t.priority) || '—'),
          h('td', {}, assigneeChips(t)),
          h('td', { text: t.estimate || '—' }),
          h('td', { text: fmtDate(t.startDate) }),
          h('td', {}, isOverdue(t) ? h('span', { class: 'due overdue', text: fmtDate(t.dueDate) }) : fmtDate(t.dueDate)))))));
    }

    function render() {
      const list = filtered();
      counter.textContent = `${list.length} de ${tasks.length} tareas`;
      content.replaceChildren(list.length === 0
        ? h('p', { class: 'muted empty-state', text: 'Ninguna tarea coincide con los filtros.' })
        : (mode === 'board' ? renderBoard(list) : renderTable(list)));
      tabBoard.classList.toggle('active', mode === 'board');
      tabTable.classList.toggle('active', mode === 'table');
      tabBoard.setAttribute('aria-selected', mode === 'board');
      tabTable.setAttribute('aria-selected', mode === 'table');
    }

    const tabBoard = h('button', { class: 'tab', type: 'button', role: 'tab', onclick: () => { mode = 'board'; render(); }, text: 'Tablero' });
    const tabTable = h('button', { class: 'tab', type: 'button', role: 'tab', onclick: () => { mode = 'table'; render(); }, text: 'Tabla' });
    [fSearch, fStatus, fPrio, fAssignee, fOverdue].forEach((el) => el.addEventListener(el.type === 'search' ? 'input' : 'change', render));

    const clearBtn = h('button', { class: 'btn btn-ghost', type: 'button', text: 'Limpiar filtros', onclick: () => {
      fSearch.value = ''; fStatus.value = '*'; fPrio.value = '*'; fAssignee.value = '*'; fOverdue.checked = false; render();
    } });
    const exportBtn = h('button', { class: 'btn btn-ghost', type: 'button', text: 'Exportar CSV', onclick: () => exportCSV(project.title, filtered()) });
    const refreshBtn = h('button', { class: 'btn', type: 'button', text: 'Actualizar desde GitHub', onclick: async () => {
      refreshBtn.disabled = true;
      try { await API.post('/api/projects/' + encodeURIComponent(id) + '/refresh'); viewProject(id); }
      catch (e) { refreshBtn.disabled = false; alert(e.message); }
    } });

    setView(
      h('a', { href: '#/', class: 'back', text: '← Proyectos' }),
      h('div', { class: 'page-head' },
        h('div', {},
          h('h1', { text: project.title }),
          h('p', { class: 'muted' }, project.shortDescription || '', project.shortDescription ? ' · ' : '',
            'Datos consultados: ' + fmtDateTime(data.fetchedAt))),
        h('div', { class: 'actions' }, refreshBtn,
          h('a', { class: 'btn btn-ghost', href: project.url, target: '_blank', rel: 'noopener noreferrer', text: 'Ver en GitHub ↗' }))),
      data.truncated ? h('div', { class: 'alert', text: 'El proyecto tiene muchas tareas; se muestran las primeras 2000.' }) : null,
      summaryView(summary),
      h('div', { class: 'filters' }, fSearch, fStatus, fPrio, fAssignee,
        h('label', { class: 'check' }, fOverdue, ' Solo vencidas')),
      h('div', { class: 'view-bar' },
        h('div', { class: 'tabs', role: 'tablist' }, tabBoard, tabTable),
        h('div', { class: 'actions' }, counter, clearBtn, exportBtn)),
      content);
    render();
  }

  function stat(label, value, cls) {
    return h('div', { class: 'stat ' + (cls || '') }, h('span', { class: 'stat-value', text: value }), h('span', { class: 'stat-label', text: label }));
  }

  function barList(title, items, total) {
    return h('div', { class: 'panel' }, h('h3', { text: title }),
      h('ul', { class: 'bar-list' }, ...items.slice(0, 8).map((it) => {
        const fill = h('span', { class: 'bar-fill' });
        fill.style.width = (total ? Math.round(it.count * 100 / total) : 0) + '%';
        return h('li', {}, h('span', { class: 'bar-name', text: it.name }), h('span', { class: 'bar' }, fill), h('span', { class: 'bar-count', text: it.count }));
      })));
  }

  function summaryView(s) {
    const fill = h('span', { class: 'progress-fill' });
    fill.style.width = s.progressPct + '%';
    return h('section', { class: 'summary', 'aria-label': 'Resumen del proyecto' },
      h('div', { class: 'stats' },
        stat('Tareas', s.total),
        stat('Completadas', s.done, 'ok'),
        stat('Abiertas', s.open),
        stat('Vencidas', s.overdue, s.overdue ? 'bad' : ''),
        stat('Vencen en 7 días', s.dueSoon, s.dueSoon ? 'warn' : ''),
        stat('Sin responsable', s.unassigned)),
      h('div', { class: 'progress-row' },
        h('span', { class: 'small', text: `Avance: ${s.progressPct}% de las tareas completadas` +
          (s.estimateAll ? ` · Estimación completada: ${s.estimateDone} de ${s.estimateAll}` : '') }),
        h('span', { class: 'progress', role: 'progressbar', 'aria-valuenow': s.progressPct, 'aria-valuemin': 0, 'aria-valuemax': 100 }, fill)),
      h('div', { class: 'panels' },
        barList('Por estado', s.byStatus, s.total),
        barList('Por responsable', s.byAssignee, s.total),
        barList('Por prioridad', s.byPriority, s.total)));
  }

  function taskDetail(t) {
    const rows = [
      ['Tipo', typeLabel[t.type] || t.type],
      ['Repositorio', t.repository ? `${t.repository}${t.number ? ' #' + t.number : ''}` : '—'],
      ['Estado (proyecto)', t.status || '—'],
      ['Estado (GitHub)', t.state || '—'],
      ['Prioridad', t.priority || '—'],
      ['Estimación', t.estimate || '—'],
      ['Fecha de inicio', fmtDate(t.startDate)],
      ['Fecha de entrega', fmtDate(t.dueDate)],
      ['Iteración', t.iteration || '—'],
      ['Última actualización', fmtDateTime(t.updatedAt)],
    ];
    const known = new Set(['Status', 'Priority', 'Estimate', 'Start date', 'Target date']);
    const extra = Object.entries(t.fields || {}).filter(([k, v]) => v && !known.has(k) && !rows.some((r) => r[1] === v));
    return h('div', {},
      h('h2', { id: 'modal-title', text: t.title }),
      h('dl', { class: 'detail' }, ...[...rows, ...extra].flatMap(([k, v]) => [h('dt', { text: k }), h('dd', { text: v })]),
        h('dt', { text: 'Responsables' }), h('dd', {}, t.assignees.length ? t.assignees.map((a) => a.name ? `${a.name} (@${a.login})` : '@' + a.login).join(', ') : 'Sin responsable'),
        h('dt', { text: 'Etiquetas' }), h('dd', {}, t.labels.length ? h('div', { class: 'chips' }, ...t.labels.map((l) => h('span', { class: 'chip', text: l }))) : '—')),
      t.url ? h('a', { class: 'btn', href: t.url, target: '_blank', rel: 'noopener noreferrer', text: 'Abrir en GitHub ↗' }) : null);
  }

  // Exporta a CSV las tareas filtradas. Neutraliza "inyección de fórmulas" en Excel/Sheets:
  // celdas que empiezan con = + - @ se prefijan con comilla simple.
  function exportCSV(title, list) {
    const cell = (v) => {
      let s = String(v == null ? '' : v);
      if (/^[=+\-@\t\r]/.test(s)) s = "'" + s;
      return '"' + s.replace(/"/g, '""') + '"';
    };
    const header = ['Título', 'Tipo', 'Repositorio', 'Número', 'Estado', 'Prioridad', 'Responsables', 'Estimación', 'Inicio', 'Entrega', 'Etiquetas', 'URL'];
    const lines = [header.map(cell).join(',')].concat(list.map((t) => [
      t.title, typeLabel[t.type] || t.type, t.repository, t.number || '', t.status, t.priority,
      t.assignees.map((a) => a.login).join('; '), t.estimate, t.startDate, t.dueDate, t.labels.join('; '), t.url,
    ].map(cell).join(',')));
    const blob = new Blob(['﻿' + lines.join('\r\n')], { type: 'text/csv;charset=utf-8' });
    const a = h('a', { download: `${slug(title) || 'proyecto'}-${todayISO()}.csv` });
    a.href = URL.createObjectURL(blob);
    document.body.append(a);
    a.click();
    setTimeout(() => { URL.revokeObjectURL(a.href); a.remove(); }, 1000);
  }

  // ---------- vista: cambiar contraseña ----------
  function viewPassword() {
    markNav('password');
    const forced = me && me.mustChangePassword;
    const err = h('p', { class: 'form-error', role: 'alert', hidden: true });
    const ok = h('p', { class: 'form-ok', role: 'status', hidden: true });
    const cur = h('input', { class: 'input', type: 'password', id: 'cur', autocomplete: 'current-password', required: true, maxlength: 128 });
    const nw = h('input', { class: 'input', type: 'password', id: 'new', autocomplete: 'new-password', required: true, minlength: 12, maxlength: 128 });
    const nw2 = h('input', { class: 'input', type: 'password', id: 'new2', autocomplete: 'new-password', required: true, minlength: 12, maxlength: 128 });
    const form = h('form', { class: 'panel form narrow', novalidate: true },
      h('h1', { text: 'Cambiar contraseña' }),
      forced ? h('div', { class: 'alert', text: 'Por seguridad debe cambiar la contraseña temporal antes de continuar.' }) : null,
      h('label', { for: 'cur', text: 'Contraseña actual' }), cur,
      h('label', { for: 'new', text: 'Nueva contraseña' }), nw,
      h('p', { class: 'small muted', text: 'Mínimo 12 caracteres, con letras y números.' }),
      h('label', { for: 'new2', text: 'Repetir nueva contraseña' }), nw2,
      err, ok,
      h('button', { class: 'btn btn-primary', type: 'submit', text: 'Guardar' }));
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      err.hidden = ok.hidden = true;
      if (nw.value !== nw2.value) { err.textContent = 'Las contraseñas nuevas no coinciden.'; err.hidden = false; return; }
      try {
        await API.post('/api/auth/password', { currentPassword: cur.value, newPassword: nw.value });
        cur.value = nw.value = nw2.value = '';
        ok.textContent = 'Contraseña actualizada.';
        ok.hidden = false;
        if (me) me.mustChangePassword = false;
        if (forced) setTimeout(() => { location.hash = '#/'; }, 800);
      } catch (ex) {
        if (ex.status === 401) return handleAuthError(ex);
        err.textContent = ex.message; err.hidden = false;
      }
    });
    setView(form);
  }

  // ---------- vista: administración ----------
  async function viewAdmin() {
    markNav('admin');
    if (!me || me.role !== 'admin') { location.hash = '#/'; return; }
    setView(h('p', { class: 'muted', text: 'Cargando…' }));
    let users, audit;
    try {
      [users, audit] = await Promise.all([API.get('/api/admin/users'), API.get('/api/admin/audit')]);
    } catch (e) {
      if (handleAuthError(e)) return;
      setView(errorBox(e, viewAdmin));
      return;
    }

    const err = h('p', { class: 'form-error', role: 'alert', hidden: true });
    const email = h('input', { class: 'input', type: 'email', placeholder: 'correo@empresa.com', required: true, maxlength: 254, 'aria-label': 'Correo' });
    const name = h('input', { class: 'input', type: 'text', placeholder: 'Nombre', required: true, maxlength: 100, 'aria-label': 'Nombre' });
    const role = h('select', { class: 'input', 'aria-label': 'Rol' }, h('option', { value: 'viewer', text: 'Consulta' }), h('option', { value: 'admin', text: 'Administrador' }));
    const pw = h('input', { class: 'input', type: 'text', placeholder: 'Contraseña temporal', required: true, minlength: 12, maxlength: 128, autocomplete: 'off', 'aria-label': 'Contraseña temporal' });
    const genBtn = h('button', { class: 'btn btn-ghost', type: 'button', text: 'Generar', onclick: () => { pw.value = randomPassword(); } });
    const form = h('form', { class: 'panel form-inline', novalidate: true },
      h('h3', { text: 'Crear usuario' }),
      h('div', { class: 'row' }, email, name, role, pw, genBtn, h('button', { class: 'btn btn-primary', type: 'submit', text: 'Crear' })),
      h('p', { class: 'small muted', text: 'El usuario deberá cambiar la contraseña temporal en su primer ingreso. Compártala por un canal seguro.' }),
      err);
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      err.hidden = true;
      try {
        await API.post('/api/admin/users', { email: email.value, name: name.value, role: role.value, temporaryPassword: pw.value });
        viewAdmin();
      } catch (ex) { if (!handleAuthError(ex)) { err.textContent = ex.message; err.hidden = false; } }
    });

    const roleName = { admin: 'Administrador', viewer: 'Consulta' };
    async function update(u, body) {
      try { await API.patch('/api/admin/users/' + encodeURIComponent(u.id), body); viewAdmin(); }
      catch (ex) { if (!handleAuthError(ex)) alert(ex.message); }
    }
    const table = h('div', { class: 'table-wrap' }, h('table', { class: 'table' },
      h('thead', {}, h('tr', {}, ...['Correo', 'Nombre', 'Rol', 'Estado', 'Último ingreso', 'Acciones'].map((t) => h('th', { scope: 'col', text: t })))),
      h('tbody', {}, ...users.users.map((u) => {
        const self = u.id === me.id;
        return h('tr', { class: u.active ? '' : 'row-muted' },
          h('td', { text: u.email }), h('td', { text: u.name }),
          h('td', { text: roleName[u.role] || u.role }),
          h('td', {}, u.active ? (u.mustChangePassword ? 'Activo (clave temporal)' : 'Activo') : 'Inactivo'),
          h('td', { text: u.lastLoginAt && !u.lastLoginAt.startsWith('0001') ? fmtDateTime(u.lastLoginAt) : 'Nunca' }),
          h('td', {}, h('div', { class: 'actions' },
            self ? h('span', { class: 'small muted', text: '(usted)' }) : [
              h('button', { class: 'btn btn-ghost btn-sm', type: 'button', text: u.role === 'admin' ? 'Hacer consulta' : 'Hacer admin',
                onclick: () => update(u, { role: u.role === 'admin' ? 'viewer' : 'admin' }) }),
              h('button', { class: 'btn btn-ghost btn-sm', type: 'button', text: u.active ? 'Desactivar' : 'Activar',
                onclick: () => { if (!u.active || confirm(`¿Desactivar a ${u.email}? Se cerrarán sus sesiones.`)) update(u, { active: !u.active }); } }),
              h('button', { class: 'btn btn-ghost btn-sm', type: 'button', text: 'Restablecer clave', onclick: () => {
                const tmp = randomPassword();
                if (confirm(`Se asignará la contraseña temporal:\n\n${tmp}\n\nCópiela antes de aceptar y compártala por un canal seguro.`)) update(u, { temporaryPassword: tmp });
              } }),
            ])));
      }))));

    const actionNames = { login: 'Ingreso', login_failed: 'Ingreso fallido', login_blocked: 'Ingreso bloqueado', logout: 'Salida',
      password_change: 'Cambio de contraseña', user_create: 'Creación de usuario', user_update: 'Modificación de usuario' };
    const auditTable = h('div', { class: 'table-wrap' }, h('table', { class: 'table table-compact' },
      h('thead', {}, h('tr', {}, ...['Fecha', 'Acción', 'Usuario', 'Detalle', 'IP', 'Resultado'].map((t) => h('th', { scope: 'col', text: t })))),
      h('tbody', {}, ...audit.entries.slice(0, 100).map((a) => h('tr', {},
        h('td', { text: fmtDateTime(a.at) }), h('td', { text: actionNames[a.action] || a.action }),
        h('td', { text: a.actorEmail || '—' }), h('td', { text: a.target || '' }), h('td', { text: a.ip }),
        h('td', {}, h('span', { class: 'badge ' + (a.success ? 'ok' : 'bad'), text: a.success ? 'OK' : 'Fallo' })))))));

    setView(
      h('div', { class: 'page-head' }, h('h1', { text: 'Administración' })),
      form,
      h('h2', { text: 'Usuarios' }), table,
      h('h2', { text: 'Bitácora de auditoría (últimos 100 eventos)' }), auditTable);
  }

  function randomPassword() {
    const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789';
    const bytes = new Uint32Array(16);
    crypto.getRandomValues(bytes);
    let s = Array.from(bytes, (b) => chars[b % chars.length]).join('');
    return s.slice(0, 12) + '-' + (bytes[0] % 90 + 10) + s.slice(12, 14);
  }

  // ---------- enrutador ----------
  function route() {
    closeModal();
    const hash = location.hash || '#/';
    if (me && me.mustChangePassword && hash !== '#/password') { location.hash = '#/password'; return; }
    const m = hash.match(/^#\/project\/([^/]+)$/);
    if (m) return viewProject(decodeURIComponent(m[1]));
    if (hash === '#/admin') return viewAdmin();
    if (hash === '#/password') return viewPassword();
    return viewProjects();
  }

  async function init() {
    try {
      const data = await API.get('/api/auth/me');
      me = data.user;
      API.setCsrf(data.csrfToken);
      document.getElementById('nav').hidden = false;
      document.getElementById('nav-admin').hidden = me.role !== 'admin';
      document.getElementById('user-label').textContent = me.name || me.email;
      document.getElementById('demo-banner').hidden = !data.demoMode;
    } catch (e) {
      window.location.replace('/login');
      return;
    }
    document.getElementById('logout-btn').addEventListener('click', async () => {
      try { await API.post('/api/auth/logout'); } catch (e) { /* ignorar */ }
      window.location.replace('/login');
    });
    window.addEventListener('hashchange', route);
    route();
  }
  init();
})();
