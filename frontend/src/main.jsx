import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Bell, Building2, CheckCircle2, Download, FileSpreadsheet, Home, LogOut, MessageSquareText, Plus, Search, Settings, ShieldCheck, Upload, Users, WalletCards } from 'lucide-react';
import './styles.css';

const API = import.meta.env.VITE_API_URL || 'http://localhost:8080/api';

function request(path, options = {}) {
  const token = localStorage.getItem('rentpulse_token');
  const headers = options.body instanceof FormData ? {} : { 'Content-Type': 'application/json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  return fetch(`${API}${path}`, { ...options, headers: { ...headers, ...(options.headers || {}) } }).then(async (res) => {
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.error || `Request failed with ${res.status}`);
    }
    const type = res.headers.get('content-type') || '';
    return type.includes('application/json') ? res.json() : res.blob();
  });
}

function App() {
  const [token, setToken] = useState(localStorage.getItem('rentpulse_token'));
  const [route, setRoute] = useState(token ? 'dashboard' : 'landing');
  const [user, setUser] = useState(null);

  useEffect(() => {
    if (!token) return;
    request('/me').then(setUser).catch(() => logout());
  }, [token]);

  function login(nextToken) {
    localStorage.setItem('rentpulse_token', nextToken);
    setToken(nextToken);
    setRoute('dashboard');
  }

  function logout() {
    localStorage.removeItem('rentpulse_token');
    setToken(null);
    setUser(null);
    setRoute('landing');
  }

  if (!token && route === 'auth') return <Auth onLogin={login} onBack={() => setRoute('landing')} />;
  if (!token) return <Landing onAuth={() => setRoute('auth')} />;

  return (
    <Shell route={route} setRoute={setRoute} user={user} logout={logout}>
      {route === 'dashboard' && <Dashboard />}
      {route === 'tenants' && <Tenants />}
      {route === 'pricing' && <Pricing />}
      {route === 'settings' && <SettingsPage />}
    </Shell>
  );
}

function Landing({ onAuth }) {
  return (
    <main className="site">
      <nav className="topnav">
        <strong className="brand">RentPulse</strong>
        <div className="navlinks"><a href="#features">Features</a><a href="#pricing">Pricing</a><a href="#trust">Trust</a></div>
        <button className="btn primary" onClick={onAuth}>Get Started</button>
      </nav>
      <section className="hero">
        <div className="heroText">
          <span className="pill">Automated rent collection for Kenyan landlords</span>
          <h1>Stop chasing rent. Start running a real property business.</h1>
          <p>RentPulse keeps tenants, due dates, reminders, payment verification, imports, exports, and monthly Excel reports in one operational system.</p>
          <div className="actions"><button className="btn primary big" onClick={onAuth}>Create account</button><a className="btn ghost big" href="#features">See features</a></div>
        </div>
        <div className="heroMedia">
          <img alt="Modern Nairobi apartment building" src="https://images.unsplash.com/photo-1545324418-cc1a3fa10c00?auto=format&fit=crop&w=1200&q=80" />
          <div className="receipt"><CheckCircle2 size={22} /><div><strong>KES 85,000 verified</strong><span>M-Pesa reference matched to Unit 4B</span></div></div>
        </div>
      </section>
      <section className="featureGrid" id="features">
        <Feature icon={<MessageSquareText />} title="Automated reminders" text="Send scheduled SMS reminders before due date, on due date, and after arrears are flagged." />
        <Feature icon={<ShieldCheck />} title="Payment verification" text="Tenants mark payments as done; landlords verify manually or through configured M-Pesa credentials." />
        <Feature icon={<Upload />} title="Bulk imports" text="Import tenants from CSV or XLSX and keep a durable job record with row-level errors." />
        <Feature icon={<FileSpreadsheet />} title="Excel reports" text="Generate monthly collection workbooks from the live ledger." />
      </section>
      <section className="pricingBand" id="pricing"><Pricing publicMode /></section>
      <footer id="trust">RentPulse © 2026. Built for property stewards who need a paper trail.</footer>
    </main>
  );
}

function Feature({ icon, title, text }) {
  return <article className="feature">{icon}<h3>{title}</h3><p>{text}</p></article>;
}

function Auth({ onLogin, onBack }) {
  const [mode, setMode] = useState('register');
  const [form, setForm] = useState({ organization_name: '', full_name: '', email: '', phone: '', password: '' });
  const [error, setError] = useState('');
  async function submit(e) {
    e.preventDefault();
    setError('');
    try {
      const path = mode === 'register' ? '/auth/register' : '/auth/login';
      const body = mode === 'register' ? form : { email: form.email, password: form.password };
      const out = await request(path, { method: 'POST', body: JSON.stringify(body) });
      onLogin(out.token);
    } catch (err) {
      setError(err.message);
    }
  }
  return (
    <main className="authPage">
      <button className="linkButton" onClick={onBack}>Back</button>
      <form className="authCard" onSubmit={submit}>
        <strong className="brand">RentPulse</strong>
        <h1>{mode === 'register' ? 'Create your workspace' : 'Welcome back'}</h1>
        {mode === 'register' && <input placeholder="Organization name" value={form.organization_name} onChange={e => setForm({ ...form, organization_name: e.target.value })} />}
        {mode === 'register' && <input placeholder="Full name" value={form.full_name} onChange={e => setForm({ ...form, full_name: e.target.value })} />}
        <input placeholder="Email" type="email" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} />
        {mode === 'register' && <input placeholder="Phone" value={form.phone} onChange={e => setForm({ ...form, phone: e.target.value })} />}
        <input placeholder="Password" type="password" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} />
        {error && <p className="error">{error}</p>}
        <button className="btn primary">{mode === 'register' ? 'Create account' : 'Sign in'}</button>
        <button type="button" className="linkButton" onClick={() => setMode(mode === 'register' ? 'login' : 'register')}>{mode === 'register' ? 'I already have an account' : 'Create a new account'}</button>
      </form>
    </main>
  );
}

function Shell({ children, route, setRoute, user, logout }) {
  const nav = [
    ['dashboard', Home, 'Dashboard'],
    ['tenants', Users, 'Tenants'],
    ['pricing', WalletCards, 'Pricing'],
    ['settings', Settings, 'Settings']
  ];
  return (
    <div className="app">
      <aside className="sidebar">
        <div><strong className="brand">RentPulse</strong><span className="tagline">The Modern Steward</span></div>
        <nav>{nav.map(([id, Icon, label]) => <button key={id} className={route === id ? 'active' : ''} onClick={() => setRoute(id)}><Icon size={20} />{label}</button>)}</nav>
        <button className="btn primary" onClick={() => setRoute('tenants')}><Plus size={18} />Add tenant</button>
      </aside>
      <section className="workspace">
        <header className="appbar">
          <div className="search"><Search size={18} /><input placeholder="Search tenants, units, payments..." /></div>
          <div className="userbar"><Bell size={20} /><span>{user?.FullName || user?.full_name || 'Owner'}</span><button onClick={logout} title="Log out"><LogOut size={18} /></button></div>
        </header>
        {children}
      </section>
    </div>
  );
}

function Dashboard() {
  const [data, setData] = useState(null);
  const [error, setError] = useState('');
  useEffect(() => { request('/dashboard').then(setData).catch(e => setError(e.message)); }, []);
  if (error) return <Page title="Dashboard"><p className="error">{error}</p></Page>;
  if (!data) return <Page title="Dashboard"><p>Loading live portfolio...</p></Page>;
  const pct = data.total_due_cents ? Math.round((data.collected_cents / data.total_due_cents) * 100) : 0;
  return (
    <Page title="Collection command center" subtitle="Live values from your ledger for the current month.">
      <div className="stats">
        <Stat label="Total due" value={kes(data.total_due_cents)} />
        <Stat label="Collected" value={kes(data.collected_cents)} tone="green" />
        <Stat label="Overdue" value={kes(data.overdue_cents)} tone="red" />
        <Stat label="Occupancy" value={`${data.occupied_units}/${data.units}`} />
      </div>
      <section className="panel">
        <div className="panelHead"><h2>Rent collection status</h2><strong>{pct}% collected</strong></div>
        <div className="progress"><span style={{ width: `${pct}%` }} /></div>
        <p>{data.pending_count} payment item(s) currently need attention.</p>
      </section>
    </Page>
  );
}

function Tenants() {
  const [tenants, setTenants] = useState([]);
  const [form, setForm] = useState({ full_name: '', phone: '', email: '' });
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const load = () => request('/tenants').then(setTenants).catch(e => setError(e.message));
  useEffect(load, []);
  async function add(e) {
    e.preventDefault();
    setError('');
    await request('/tenants', { method: 'POST', body: JSON.stringify(form) }).then(() => { setForm({ full_name: '', phone: '', email: '' }); load(); }).catch(e => setError(e.message));
  }
  async function upload(e) {
    const file = e.target.files[0];
    if (!file) return;
    const body = new FormData();
    body.append('file', file);
    const out = await request('/imports/tenants', { method: 'POST', body }).catch(e => setError(e.message));
    if (out) { setNotice(`Imported ${out.imported_rows} of ${out.total_rows} rows`); load(); }
  }
  return (
    <Page title="Resident directory" subtitle="Tenant records come from the API. Add one manually or import a CSV/XLSX file.">
      <form className="inlineForm" onSubmit={add}>
        <input placeholder="Full name" value={form.full_name} onChange={e => setForm({ ...form, full_name: e.target.value })} />
        <input placeholder="Phone" value={form.phone} onChange={e => setForm({ ...form, phone: e.target.value })} />
        <input placeholder="Email" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} />
        <button className="btn primary"><Plus size={18} />Add</button>
      </form>
      <div className="toolbar">
        <label className="btn ghost"><Upload size={18} />Import<input type="file" accept=".csv,.xlsx" onChange={upload} hidden /></label>
        <a className="btn ghost" href={`${API}/exports/tenants.csv`} onClick={(e) => attachTokenDownload(e, '/exports/tenants.csv')}><Download size={18} />Export CSV</a>
        <a className="btn ghost" href={`${API}/reports/monthly.xlsx`} onClick={(e) => attachTokenDownload(e, '/reports/monthly.xlsx')}><FileSpreadsheet size={18} />Excel report</a>
      </div>
      {error && <p className="error">{error}</p>}{notice && <p className="notice">{notice}</p>}
      <section className="panel tablePanel">
        {tenants.length === 0 ? <Empty title="No tenants yet" text="Create or import tenants to populate your ledger." /> : <table><thead><tr><th>Name</th><th>Phone</th><th>Email</th><th>Property</th><th>Unit</th><th>Rent</th></tr></thead><tbody>{tenants.map(t => <tr key={t.id}><td>{t.full_name}</td><td>{t.phone}</td><td>{t.email}</td><td>{t.property_name}</td><td>{t.unit_label}</td><td>{kes(t.rent_cents || 0)}</td></tr>)}</tbody></table>}
      </section>
    </Page>
  );
}

function Pricing({ publicMode = false }) {
  const [plans, setPlans] = useState([]);
  useEffect(() => { request('/plans').then(setPlans).catch(() => setPlans([])); }, []);
  return (
    <Page title={publicMode ? 'Plans that match your portfolio' : 'Pricing'} subtitle={publicMode ? '' : 'Plan data is served by the backend.'}>
      <div className="plans">{plans.map(plan => <article className={plan.id === 'pro' ? 'plan popular' : 'plan'} key={plan.id}><span className="pill">{plan.name}</span><h2>{plan.price_cents === null ? 'Custom' : kes(plan.price_cents)}<small>{plan.price_cents === null ? '' : '/mo'}</small></h2><p>{plan.unit_limit ? `Up to ${plan.unit_limit} units` : 'Unlimited units'}</p>{plan.features.map(f => <div className="check" key={f}><CheckCircle2 size={17} />{f}</div>)}</article>)}</div>
    </Page>
  );
}

function SettingsPage() {
  const [form, setForm] = useState(null);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  useEffect(() => { request('/settings').then(setForm).catch(e => setError(e.message)); }, []);
  async function save(e) {
    e.preventDefault();
    setNotice('');
    await request('/settings', { method: 'PUT', body: JSON.stringify(form) }).then(() => setNotice('Settings saved')).catch(e => setError(e.message));
  }
  async function reminders() {
    const out = await request('/communications/reminders/run', { method: 'POST', body: '{}' }).catch(e => setError(e.message));
    if (out) setNotice(`Reminder run complete: ${out.sent} sent, ${out.skipped} skipped, ${out.failed} failed`);
  }
  if (!form) return <Page title="Settings"><p>{error || 'Loading settings...'}</p></Page>;
  return (
    <Page title="Settings" subtitle="Configure payment details and communication templates used by the reminder algorithm.">
      <form className="settingsForm" onSubmit={save}>
        <label>M-Pesa Paybill<input value={form.mpesa_paybill} onChange={e => setForm({ ...form, mpesa_paybill: e.target.value })} /></label>
        <label>M-Pesa Till<input value={form.mpesa_till} onChange={e => setForm({ ...form, mpesa_till: e.target.value })} /></label>
        <label>SMS sender ID<input value={form.sms_sender_id} onChange={e => setForm({ ...form, sms_sender_id: e.target.value })} /></label>
        <label>Reminder before days<input type="number" value={form.reminder_before_days} onChange={e => setForm({ ...form, reminder_before_days: Number(e.target.value) })} /></label>
        <label>Reminder template<textarea value={form.reminder_template} onChange={e => setForm({ ...form, reminder_template: e.target.value })} /></label>
        <label>Escalation template<textarea value={form.escalation_template} onChange={e => setForm({ ...form, escalation_template: e.target.value })} /></label>
        <div className="toolbar"><button className="btn primary">Save settings</button><button type="button" className="btn ghost" onClick={reminders}><MessageSquareText size={18} />Run reminders</button></div>
      </form>
      {notice && <p className="notice">{notice}</p>}{error && <p className="error">{error}</p>}
    </Page>
  );
}

function Page({ title, subtitle, children }) {
  return <main className="page"><div className="pageHead"><h1>{title}</h1>{subtitle && <p>{subtitle}</p>}</div>{children}</main>;
}

function Stat({ label, value, tone = '' }) {
  return <article className={`stat ${tone}`}><span>{label}</span><strong>{value}</strong></article>;
}

function Empty({ title, text }) {
  return <div className="empty"><Building2 size={34} /><h3>{title}</h3><p>{text}</p></div>;
}

function kes(cents) {
  return `KES ${Number(cents / 100).toLocaleString('en-KE', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

async function attachTokenDownload(e, path) {
  e.preventDefault();
  const blob = await request(path);
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = path.endsWith('.xlsx') ? 'rentpulse-monthly-report.xlsx' : 'rentpulse-tenants.csv';
  a.click();
  URL.revokeObjectURL(url);
}

createRoot(document.getElementById('root')).render(<App />);
