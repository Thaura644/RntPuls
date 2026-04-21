import React, { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Bell, Building2, CheckCircle2, Download, FileSpreadsheet, Home, LogOut, MessageSquareText, Plus, Search, Settings, ShieldCheck, Upload, Users, WalletCards } from 'lucide-react';
import './styles.css';

const API = import.meta.env.VITE_API_URL || 'http://localhost:8080/api';

function request(path, options = {}) {
  const token = options.token || localStorage.getItem('rentpulse_token');
  const headers = options.body instanceof FormData ? {} : { 'Content-Type': 'application/json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  const { token: _token, ...fetchOptions } = options;
  return fetch(`${API}${path}`, { ...fetchOptions, headers: { ...headers, ...(options.headers || {}) } }).then(async (res) => {
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.error || `Request failed with ${res.status}`);
    }
    const type = res.headers.get('content-type') || '';
    return type.includes('application/json') ? res.json() : res.blob();
  });
}

function App() {
  const tenantToken = new URLSearchParams(window.location.search).get('token');
  if (window.location.pathname === '/tenant' && tenantToken) {
    return <TenantPortal token={tenantToken} />;
  }

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
    <ErrorBoundary route={route}>
      <Shell route={route} setRoute={setRoute} user={user} logout={logout}>
        {route === 'dashboard' && <Dashboard />}
        {route === 'tenants' && <Tenants />}
        {route === 'payments' && <Payments />}
        {route === 'settings' && <SettingsPage />}
      </Shell>
    </ErrorBoundary>
  );
}

class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { error: null };
  }

  static getDerivedStateFromError(error) {
    return { error };
  }

  componentDidUpdate(prevProps) {
    if (prevProps.route !== this.props.route && this.state.error) {
      this.setState({ error: null });
    }
  }

  render() {
    if (this.state.error) {
      return (
        <main className="authPage">
          <section className="authCard">
            <strong className="brand">RentPulse</strong>
            <h1>Something needs attention</h1>
            <p className="error">{this.state.error.message}</p>
            <button className="btn primary" onClick={() => window.location.reload()}>Reload</button>
          </section>
        </main>
      );
    }
    return this.props.children;
  }
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
      if (mode === 'register' && err.message.includes('already registered')) {
        setError('That email already has an account. Switch to sign in below.');
      } else {
        setError(err.message);
      }
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
    ['payments', WalletCards, 'Payments'],
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
  const [importOpen, setImportOpen] = useState(false);
  const load = () => {
    setError('');
    return request('/tenants')
      .then(data => setTenants(Array.isArray(data) ? data : []))
      .catch(e => setError(e.message));
  };
  useEffect(() => {
    load();
  }, []);
  async function add(e) {
    e.preventDefault();
    setError('');
    await request('/tenants', { method: 'POST', body: JSON.stringify(form) }).then(() => { setForm({ full_name: '', phone: '', email: '' }); load(); }).catch(e => setError(e.message));
  }
  async function copyTenantLink(tenantID) {
    setError('');
    const out = await request(`/tenants/${tenantID}/access-link`, { method: 'POST', body: '{}' }).catch(e => setError(e.message));
    if (!out) return;
    await navigator.clipboard.writeText(out.url);
    setNotice('Tenant portal link copied. Send it by SMS or WhatsApp.');
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
        <a className="btn ghost" href={`${API}/imports/tenants/template.csv`} onClick={(e) => attachTokenDownload(e, '/imports/tenants/template.csv', 'rentpulse-tenant-import-template.csv')}><Download size={18} />Template CSV</a>
        <button className="btn ghost" type="button" onClick={() => setImportOpen(true)}><Upload size={18} />Import wizard</button>
        <a className="btn ghost" href={`${API}/exports/tenants.csv`} onClick={(e) => attachTokenDownload(e, '/exports/tenants.csv')}><Download size={18} />Export CSV</a>
        <a className="btn ghost" href={`${API}/reports/monthly.xlsx`} onClick={(e) => attachTokenDownload(e, '/reports/monthly.xlsx')}><FileSpreadsheet size={18} />Excel report</a>
      </div>
      {error && <p className="error">{error}</p>}{notice && <p className="notice">{notice}</p>}
      <section className="panel tablePanel">
        {tenants.length === 0 ? <Empty title="No tenants yet" text="Create or import tenants to populate your ledger." /> : <table><thead><tr><th>Name</th><th>Phone</th><th>Email</th><th>Property</th><th>Unit</th><th>Rent</th><th>Tenant access</th></tr></thead><tbody>{tenants.map(t => <tr key={t.id}><td>{t.full_name}</td><td>{t.phone}</td><td>{t.email}</td><td>{t.property_name}</td><td>{t.unit_label}</td><td>{kes(t.rent_cents || 0)}</td><td><button className="linkButton" onClick={() => copyTenantLink(t.id)}>Copy link</button></td></tr>)}</tbody></table>}
      </section>
      {importOpen && <ImportWizard onClose={() => setImportOpen(false)} onImported={(summary) => { setNotice(`Imported ${summary.imported_rows} of ${summary.total_rows} rows`); setImportOpen(false); load(); }} />}
    </Page>
  );
}

const importFields = [
  ['full_name', 'Tenant full name', true],
  ['phone', 'Phone number', true],
  ['email', 'Email', false],
  ['national_id', 'National ID', false],
  ['property_name', 'Property name', false],
  ['property_address', 'Property address', false],
  ['city', 'City', false],
  ['unit_label', 'Unit label', false],
  ['monthly_rent_kes', 'Monthly rent KES', false],
  ['lease_start_date', 'Lease start date', false],
  ['due_day', 'Rent due day', false],
  ['deposit_kes', 'Deposit KES', false]
];

function ImportWizard({ onClose, onImported }) {
  const [file, setFile] = useState(null);
  const [preview, setPreview] = useState(null);
  const [mapping, setMapping] = useState({});
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function previewFile(nextFile) {
    if (!nextFile) return;
    setFile(nextFile);
    setPreview(null);
    setError('');
    setBusy(true);
    const body = new FormData();
    body.append('file', nextFile);
    const out = await request('/imports/tenants/preview', { method: 'POST', body }).catch(e => setError(e.message));
    if (out) {
      setPreview(out);
      setMapping(out.suggested_mapping || {});
    }
    setBusy(false);
  }

  async function runImport() {
    if (!file) {
      setError('Choose a CSV or XLSX file first.');
      return;
    }
    setBusy(true);
    setError('');
    const body = new FormData();
    body.append('file', file);
    body.append('mapping', JSON.stringify(mapping));
    const out = await request('/imports/tenants', { method: 'POST', body }).catch(e => setError(e.message));
    setBusy(false);
    if (out) onImported(out);
  }

  const validation = preview?.validation;
  return (
    <div className="modalBackdrop">
      <section className="importModal">
        <div className="modalHead">
          <div><h2>Tenant import wizard</h2><p>Upload a CSV or XLSX file, match columns, test the mapping, then import.</p></div>
          <button className="linkButton" onClick={onClose}>Close</button>
        </div>
        <div className="importSteps">
          <label className="dropZone">
            <Upload size={24} />
            <strong>{file ? file.name : 'Choose tenant file'}</strong>
            <span>CSV or XLSX with a header row</span>
            <input type="file" accept=".csv,.xlsx" hidden onChange={(e) => previewFile(e.target.files[0])} />
          </label>
          {preview && (
            <>
              <div className="mappingGrid">
                {importFields.map(([field, label, required]) => (
                  <label key={field}>
                    <span>{label}{required ? ' *' : ''}</span>
                    <select value={mapping[field] || ''} onChange={(e) => setMapping({ ...mapping, [field]: e.target.value })}>
                      <option value="">Do not import</option>
                      {preview.headers.map(header => <option key={header} value={header}>{header}</option>)}
                    </select>
                  </label>
                ))}
              </div>
              <div className="validationBox">
                <strong>{validation.valid_rows} valid sample row(s), {validation.invalid_rows} invalid sample row(s)</strong>
                {validation.errors.slice(0, 6).map(err => <p className="error" key={err}>{err}</p>)}
              </div>
              <div className="previewTable">
                <table>
                  <thead><tr>{preview.headers.map(header => <th key={header}>{header}</th>)}</tr></thead>
                  <tbody>{preview.sample_rows.map((row, i) => <tr key={i}>{preview.headers.map((header, idx) => <td key={header}>{row[idx]}</td>)}</tr>)}</tbody>
                </table>
              </div>
            </>
          )}
          {error && <p className="error">{error}</p>}
          <div className="toolbar">
            <button className="btn primary" type="button" onClick={runImport} disabled={busy || !preview}>{busy ? 'Working...' : 'Import tenants'}</button>
            <a className="btn ghost" href={`${API}/imports/tenants/template.csv`} onClick={(e) => attachTokenDownload(e, '/imports/tenants/template.csv', 'rentpulse-tenant-import-template.csv')}>Download template</a>
          </div>
        </div>
      </section>
    </div>
  );
}

function TenantPortal({ token }) {
  const [data, setData] = useState(null);
  const [selected, setSelected] = useState(null);
  const [form, setForm] = useState({ provider: 'mpesa', transaction_ref: '', evidence_url: '' });
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');

  function load() {
    setError('');
    return request('/tenant/me', { token }).then(setData).catch(e => setError(e.message));
  }

  useEffect(() => {
    load();
  }, [token]);

  async function upload(e) {
    const file = e.target.files[0];
    if (!file) return;
    const body = new FormData();
    body.append('file', file);
    const out = await request('/tenant/uploads', { method: 'POST', body, token }).catch(e => setError(e.message));
    if (out) {
      setForm(current => ({ ...current, evidence_url: out.url }));
      setNotice('Screenshot uploaded.');
    }
  }

  async function submit(e) {
    e.preventDefault();
    if (!selected) {
      setError('Select a payment first.');
      return;
    }
    const body = { ...form, payment_intent_id: selected.id, amount_cents: selected.amount_cents };
    const out = await request('/tenant/payments/mark-paid', { method: 'POST', body: JSON.stringify(body), token }).catch(e => setError(e.message));
    if (out) {
      setNotice('Payment submitted for landlord verification.');
      setForm({ provider: 'mpesa', transaction_ref: '', evidence_url: '' });
      setSelected(null);
      load();
    }
  }

  return (
    <main className="tenantPortal">
      <section className="tenantHero">
        <strong className="brand">RentPulse</strong>
        <h1>{data ? `Hello, ${data.full_name}` : 'Tenant payment portal'}</h1>
        <p>Upload payment evidence and transaction references directly to your landlord. Your landlord still verifies the record before it is marked collected.</p>
      </section>
      {error && <p className="error">{error}</p>}{notice && <p className="notice">{notice}</p>}
      <section className="tenantGrid">
        <div className="panel">
          <h2>Open payments</h2>
          {!data ? <p>Loading...</p> : data.payments.length === 0 ? <Empty title="No open payments" text="There are no due or overdue rent items assigned to this portal link." /> : data.payments.map(payment => (
            <button key={payment.id} className={selected?.id === payment.id ? 'paymentChoice selected' : 'paymentChoice'} onClick={() => setSelected(payment)}>
              <span>{payment.property_name} {payment.unit_label}</span>
              <strong>{kes(payment.amount_cents)}</strong>
              <small>Due {new Date(payment.due_on).toLocaleDateString()} · {payment.status}</small>
            </button>
          ))}
        </div>
        <form className="panel tenantForm" onSubmit={submit}>
          <h2>Submit proof</h2>
          <label>Payment method<input value={form.provider} onChange={e => setForm({ ...form, provider: e.target.value })} /></label>
          <label>Transaction code<input value={form.transaction_ref} onChange={e => setForm({ ...form, transaction_ref: e.target.value })} placeholder="e.g. RKP82LL09S" /></label>
          <label className="btn ghost uploadButton"><Upload size={18} />Upload screenshot or PDF<input type="file" accept=".jpg,.jpeg,.png,.pdf" hidden onChange={upload} /></label>
          {form.evidence_url && <a className="evidenceLink" href={form.evidence_url} target="_blank">Evidence uploaded</a>}
          <button className="btn primary">Submit for verification</button>
        </form>
      </section>
    </main>
  );
}

function Payments() {
  const [payments, setPayments] = useState([]);
  const [error, setError] = useState('');
  useEffect(() => {
    request('/payments').then(data => setPayments(Array.isArray(data) ? data : [])).catch(e => setError(e.message));
  }, []);
  return (
    <Page title="Payments" subtitle="Track due, overdue, submitted, and verified rent payments from the live ledger.">
      {error && <p className="error">{error}</p>}
      <section className="panel tablePanel">
        {payments.length === 0 ? <Empty title="No payments yet" text="Payment items are created when active leases exist for the current month." /> : (
          <table>
            <thead><tr><th>Tenant</th><th>Unit</th><th>Due on</th><th>Amount</th><th>Status</th><th>Reference</th><th>Evidence</th></tr></thead>
            <tbody>{payments.map(payment => (
              <tr key={payment.id}>
                <td>{payment.tenant_name}<br /><small>{payment.tenant_phone}</small></td>
                <td>{payment.property_name} {payment.unit_label}</td>
                <td>{payment.due_on ? new Date(payment.due_on).toLocaleDateString() : ''}</td>
                <td>{kes(payment.amount_cents || 0)}</td>
                <td><span className={`status ${payment.status}`}>{payment.status}</span></td>
                <td>{payment.transaction_ref}</td>
                <td>{payment.evidence_url ? <a className="evidenceLink" href={payment.evidence_url} target="_blank">Open</a> : ''}</td>
              </tr>
            ))}</tbody>
          </table>
        )}
      </section>
    </Page>
  );
}

function Pricing({ publicMode = false }) {
  const [plans, setPlans] = useState([]);
  useEffect(() => { request('/plans').then(data => setPlans(Array.isArray(data) ? data : [])).catch(() => setPlans([])); }, []);
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
  const [tab, setTab] = useState('general');
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
    <Page title="Settings" subtitle={`Current plan: ${form.plan}. Configure operations, properties, units, and billing.`}>
      <div className="tabs">
        {['general', 'properties', 'pricing'].map(id => <button key={id} className={tab === id ? 'active' : ''} onClick={() => setTab(id)}>{id}</button>)}
      </div>
      {tab === 'general' && (
        <form className="settingsForm" onSubmit={save}>
          <label>M-Pesa Paybill<input value={form.mpesa_paybill} onChange={e => setForm({ ...form, mpesa_paybill: e.target.value })} /></label>
          <label>M-Pesa Till<input value={form.mpesa_till} onChange={e => setForm({ ...form, mpesa_till: e.target.value })} /></label>
          <label>SMS sender ID<input value={form.sms_sender_id} onChange={e => setForm({ ...form, sms_sender_id: e.target.value })} /></label>
          <label>Reminder before days<input type="number" value={form.reminder_before_days} onChange={e => setForm({ ...form, reminder_before_days: Number(e.target.value) })} /></label>
          <label>Reminder template<textarea value={form.reminder_template} onChange={e => setForm({ ...form, reminder_template: e.target.value })} /></label>
          <label>Escalation template<textarea value={form.escalation_template} onChange={e => setForm({ ...form, escalation_template: e.target.value })} /></label>
          <div className="toolbar"><button className="btn primary">Save settings</button><button type="button" className="btn ghost" onClick={reminders}><MessageSquareText size={18} />Run reminders</button></div>
        </form>
      )}
      {tab === 'properties' && <PropertyManager />}
      {tab === 'pricing' && <Pricing publicMode />}
      {notice && <p className="notice">{notice}</p>}{error && <p className="error">{error}</p>}
    </Page>
  );
}

function PropertyManager() {
  const [properties, setProperties] = useState([]);
  const [units, setUnits] = useState([]);
  const [propertyForm, setPropertyForm] = useState({ name: '', address: '', city: '' });
  const [unitForm, setUnitForm] = useState({ property_id: '', label: '', rent_cents: '' });
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const load = () => {
    request('/properties').then(data => setProperties(Array.isArray(data) ? data : [])).catch(e => setError(e.message));
    request('/units').then(data => setUnits(Array.isArray(data) ? data : [])).catch(e => setError(e.message));
  };
  useEffect(load, []);
  async function createProperty(e) {
    e.preventDefault();
    setError('');
    const out = await request('/properties', { method: 'POST', body: JSON.stringify({ ...propertyForm, units: [] }) }).catch(e => setError(e.message));
    if (out) {
      setNotice('Property created');
      setPropertyForm({ name: '', address: '', city: '' });
      load();
    }
  }
  async function createUnit(e) {
    e.preventDefault();
    setError('');
    const out = await request('/units', { method: 'POST', body: JSON.stringify({ ...unitForm, rent_cents: Math.round(Number(unitForm.rent_cents) * 100) }) }).catch(e => setError(e.message));
    if (out) {
      setNotice('Unit created');
      setUnitForm({ property_id: '', label: '', rent_cents: '' });
      load();
    }
  }
  return (
    <section className="settingsSplit">
      <form className="panel tenantForm" onSubmit={createProperty}>
        <h2>Properties</h2>
        <label>Name<input value={propertyForm.name} onChange={e => setPropertyForm({ ...propertyForm, name: e.target.value })} /></label>
        <label>Address<input value={propertyForm.address} onChange={e => setPropertyForm({ ...propertyForm, address: e.target.value })} /></label>
        <label>City<input value={propertyForm.city} onChange={e => setPropertyForm({ ...propertyForm, city: e.target.value })} /></label>
        <button className="btn primary">Add property</button>
      </form>
      <form className="panel tenantForm" onSubmit={createUnit}>
        <h2>Units</h2>
        <label>Property<select value={unitForm.property_id} onChange={e => setUnitForm({ ...unitForm, property_id: e.target.value })}><option value="">Choose property</option>{properties.map(property => <option key={property.id} value={property.id}>{property.name}</option>)}</select></label>
        <label>Unit label<input value={unitForm.label} onChange={e => setUnitForm({ ...unitForm, label: e.target.value })} /></label>
        <label>Monthly rent KES<input type="number" value={unitForm.rent_cents} onChange={e => setUnitForm({ ...unitForm, rent_cents: e.target.value })} /></label>
        <button className="btn primary">Add unit</button>
      </form>
      <section className="panel tablePanel settingsWide">
        {error && <p className="error">{error}</p>}{notice && <p className="notice">{notice}</p>}
        {units.length === 0 ? <Empty title="No units yet" text="Add properties and units here, or create them through the import wizard." /> : (
          <table><thead><tr><th>Property</th><th>Unit</th><th>Rent</th><th>Status</th></tr></thead><tbody>{units.map(unit => <tr key={unit.id}><td>{unit.property_name}</td><td>{unit.label}</td><td>{kes(unit.monthly_rent_cents || 0)}</td><td>{unit.status}</td></tr>)}</tbody></table>
        )}
      </section>
    </section>
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

async function attachTokenDownload(e, path, filename) {
  e.preventDefault();
  const blob = await request(path);
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename || (path.endsWith('.xlsx') ? 'rentpulse-monthly-report.xlsx' : 'rentpulse-tenants.csv');
  a.click();
  URL.revokeObjectURL(url);
}

createRoot(document.getElementById('root')).render(<App />);
