import React, { useEffect, useState, useCallback } from 'react';
import { createRoot } from 'react-dom/client';
import { Bell, Building2, CheckCircle2, Download, FileSpreadsheet, Home, LogOut, MessageSquareText, Plus, Search, Settings, ShieldCheck, Upload, Users, WalletCards, X } from 'lucide-react';
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
        <main className="site">
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
  const scrollTo = (id) => {
    const el = document.getElementById(id);
    if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
  };

  const plans = [
    { id: 'free', name: 'Free', price: '0', period: '/mo', limit: 2, features: ['Tenant directory', 'Manual payment tracking', '2 units max'], cta: 'Start free' },
    { id: 'starter', name: 'Starter', price: '500', period: '/mo', limit: 2, features: ['Automated invoices', 'Basic rent tracking', 'CSV exports', 'Import wizard', '2 units max'], cta: 'Get started' },
    { id: 'pro', name: 'Pro', price: '1,200', period: '/mo', limit: 10, features: ['WhatsApp/SMS reminders', 'M-Pesa verification', 'Excel reports', 'Bulk imports', 'Up to 10 units'], popular: true, cta: 'Start trial' },
    { id: 'agency', name: 'Agency', price: 'Custom', period: '', limit: '∞', features: ['Unlimited units', 'Multi-user access', 'Custom branding', 'Priority support', 'Dedicated account manager'], cta: 'Contact sales' },
  ];

  return (
    <main className="site">
      <nav className="topnav">
        <strong className="brand">RentPulse</strong>
        <div className="navlinks">
          <button className="navLink" onClick={() => scrollTo('features')}>Features</button>
          <button className="navLink" onClick={() => scrollTo('how')}>How it works</button>
          <button className="navLink" onClick={() => scrollTo('pricing')}>Pricing</button>
          <button className="navLink" onClick={() => scrollTo('faq')}>FAQ</button>
        </div>
        <button className="btn primary" onClick={onAuth}>Sign in</button>
      </nav>

      <section className="hero">
        <div className="heroText">
          <span className="pill">Automated rent collection for Kenyan landlords</span>
          <h1>Stop chasing rent.<br />Start running a business.</h1>
          <p>RentPulse keeps tenants, due dates, reminders, payment verification, and monthly reports in one system. No more spreadsheets, no more missed payments.</p>
          <div className="actions">
            <button className="btn primary big" onClick={onAuth}>Create free account</button>
            <button className="btn ghost big" onClick={() => scrollTo('pricing')}>See pricing</button>
          </div>
          <div className="heroStats">
            <div className="heroStat"><strong>2,400+</strong><span>Units managed</span></div>
            <div className="heroStat"><strong>98%</strong><span>Collection rate</span></div>
            <div className="heroStat"><strong>150+</strong><span>Active landlords</span></div>
          </div>
        </div>
        <div className="heroMedia">
          <div className="heroVisual">
            <div className="dashPreview">
              <div className="dashHeader"><span>RentPulse Dashboard</span><div className="dashDots"><span /><span /><span /></div></div>
              <div className="dashStats"><div className="miniStat green">KES 850,000 <small>Collected</small></div><div className="miniStat red">KES 120,000 <small>Overdue</small></div><div className="miniStat">48/52 <small>Occupied</small></div></div>
              <div className="dashRow"><span className="dashTenant">Amina Mwangi</span><span className="dashUnit">KG-4B</span><span className="dashStatus verified">Verified</span><span className="dashAmt">KES 45,000</span></div>
              <div className="dashRow"><span className="dashTenant">Brian Otieno</span><span className="dashUnit">RH-12A</span><span className="dashStatus overdue">Overdue</span><span className="dashAmt">KES 38,000</span></div>
              <div className="dashRow"><span className="dashTenant">Faith Wanjiku</span><span className="dashUnit">MR-7C</span><span className="dashStatus pending">Pending</span><span className="dashAmt">KES 52,000</span></div>
              <div className="dashRow"><span className="dashTenant">James Kiprop</span><span className="dashUnit">KG-1A</span><span className="dashStatus verified">Verified</span><span className="dashAmt">KES 42,000</span></div>
            </div>
            <div className="floatingCard card1"><CheckCircle2 size={18} /><div><strong>KES 45,000</strong><span>M-Pesa verified</span></div></div>
            <div className="floatingCard card2"><MessageSquareText size={18} /><div><strong>SMS sent</strong><span>Reminder delivered</span></div></div>
          </div>
        </div>
      </section>

      <section className="featureGrid" id="features">
        <Feature icon={<MessageSquareText />} title="Automated SMS reminders" text="Tenants get reminders 3 days before rent is due, on the due date, and after arrears are flagged. No more awkward phone calls." />
        <Feature icon={<ShieldCheck />} title="M-Pesa verification" text="Cross-check tenant payment references against Daraja API. Know instantly if a transaction is real or fabricated." />
        <Feature icon={<Upload />} title="Bulk CSV/XLSX import" text="Onboard 50+ tenants in minutes with our smart import wizard. It maps columns automatically and validates every row." />
        <Feature icon={<FileSpreadsheet />} title="Monthly Excel reports" text="Generate professional collection workbooks with one click. Ready for your accountant or board meeting." />
        <Feature icon={<Users />} title="Tenant self-service portal" text="Share a link. Tenants upload payment screenshots and transaction codes directly. No landlord account needed." />
        <Feature icon={<Building2 />} title="Multi-property management" text="Manage Kilimani Gardens, Mombasa Road Lofts, and Riverside Heights from one dashboard. Filter by property or view all." />
      </section>

      <section className="howItWorks" id="how">
        <h2>How RentPulse works</h2>
        <p className="sectionSub">Three steps from signup to full automation</p>
        <div className="steps">
          <div className="step"><div className="stepNum">1</div><h3>Create your workspace</h3><p>Sign up in 30 seconds. Add your properties and units — or bulk import from a spreadsheet.</p></div>
          <div className="step"><div className="stepNum">2</div><h3>Add your tenants</h3><p>Import tenant records or add them manually. Assign units, set rent amounts, and choose payment methods.</p></div>
          <div className="step"><div className="stepNum">3</div><h3>Automate &amp; collect</h3><p>RentPulse generates payment intents, sends SMS reminders, and tracks every transaction — verified or pending.</p></div>
        </div>
      </section>

      <section className="pricingBand" id="pricing">
        <h2>Plans for every portfolio size</h2>
        <p className="sectionSub">Start free. Upgrade when you're ready. No credit card required.</p>
        <div className="plans">
          {plans.map(plan => (
            <article className={plan.popular ? 'plan popular' : 'plan'} key={plan.id}>
              {plan.popular && <div className="popularBadge">Most popular</div>}
              <span className="planName">{plan.name}</span>
              <div className="planPrice">{plan.price !== 'Custom' && <small>KES</small>}{plan.price}<small>{plan.period}</small></div>
              <div className="planLimit">{plan.limit === '∞' ? 'Unlimited units' : `Up to ${plan.limit} unit${plan.limit > 1 ? 's' : ''}`}</div>
              <ul className="planFeatures">{plan.features.map(f => <li key={f}><CheckCircle2 size={16} />{f}</li>)}</ul>
              <button className={plan.popular ? 'btn primary big' : 'btn ghost big'} onClick={onAuth}>{plan.cta}</button>
            </article>
          ))}
        </div>
      </section>

      <section className="trustBand" id="faq">
        <h2>Frequently asked questions</h2>
        <div className="faqGrid">
          <FAQ q="Do I need M-Pesa Daraja credentials?" a="No. RentPulse works without them. You manually verify payments. Add Daraja credentials later to automate M-Pesa transaction verification." />
          <FAQ q="Can tenants pay through the app?" a="Tenants use a portal link to submit payment references and upload screenshots. Actual payment happens via their bank or M-Pesa — RentPulse tracks and verifies." />
          <FAQ q="What happens when I exceed my unit limit?" a="You'll see a prompt to upgrade. Your existing data is never deleted. Upgrade instantly and continue without interruption." />
          <FAQ q="Is my data secure?" a="All data is encrypted at rest in PostgreSQL. Passwords are hashed with bcrypt. JWT tokens expire after 15 minutes with refresh token rotation." />
          <FAQ q="Can I export my data?" a="Yes. Export tenants to CSV, generate monthly Excel reports, and access the full API for programmatic data access." />
          <FAQ q="Do you offer a free trial of Pro?" a="Yes. Start on the Free plan and upgrade to Pro anytime. You get full access to SMS reminders, M-Pesa verification, and Excel reports." />
        </div>
      </section>

      <section className="ctaBand">
        <h2>Ready to stop chasing rent?</h2>
        <p>Create your free account and start collecting rent like a professional property business.</p>
        <button className="btn primary big" onClick={onAuth}>Create free account</button>
      </section>

      <footer>
        <div className="footerContent">
          <div><strong className="brand">RentPulse</strong><p>The Modern Steward</p></div>
          <div className="footerLinks"><div><strong>Product</strong><button className="footerLink" onClick={() => scrollTo('features')}>Features</button><button className="footerLink" onClick={() => scrollTo('pricing')}>Pricing</button></div><div><strong>Resources</strong><span>Tenant portal guide</span><span>Import template</span><span>API docs</span></div></div>
        </div>
        <div className="footerBottom">RentPulse © 2026. Built for property stewards who need a paper trail.</div>
      </footer>
    </main>
  );
}

function Feature({ icon, title, text }) {
  return <article className="feature">{icon}<h3>{title}</h3><p>{text}</p></article>;
}

function FAQ({ q, a }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="faqItem" onClick={() => setOpen(!open)}>
      <div className="faqQ"><strong>{q}</strong><span className="faqToggle">{open ? '−' : '+'}</span></div>
      {open && <p>{a}</p>}
    </div>
  );
}

function Auth({ onLogin, onBack }) {
  const [mode, setMode] = useState('register');
  const [form, setForm] = useState({ organization_name: '', full_name: '', email: '', phone: '', password: '' });
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  async function submit(e) {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      const path = mode === 'register' ? '/auth/register' : '/auth/login';
      const body = mode === 'register' ? form : { email: form.email, password: form.password };
      const out = await request(path, { method: 'POST', body: JSON.stringify(body) });
      onLogin(out.token);
    } catch (err) {
      if (err.message.includes('already registered')) {
        setError('That email already has an account. Switch to sign in below.');
      } else if (err.message.includes('too many')) {
        setError('Too many attempts. Please wait a moment and try again.');
      } else {
        setError(err.message);
      }
    } finally {
      setLoading(false);
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
        <button className="btn primary" disabled={loading}>{loading ? 'Please wait...' : (mode === 'register' ? 'Create account' : 'Sign in')}</button>
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
  const [search, setSearch] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  return (
    <div className="app">
      <aside className="sidebar">
        <div><strong className="brand">RentPulse</strong><span className="tagline">The Modern Steward</span></div>
        <nav>{nav.map(([id, Icon, label]) => <button key={id} className={route === id ? 'active' : ''} onClick={() => setRoute(id)}><Icon size={20} />{label}</button>)}</nav>
        <button className="btn primary" onClick={() => setRoute('tenants')}><Plus size={18} />Add tenant</button>
      </aside>
      <section className="workspace">
        <header className="appbar">
          <div className="search">
            {searchOpen ? (
              <div className="searchActive">
                <Search size={18} />
                <input autoFocus placeholder="Search tenants, units, payments..." value={search} onChange={e => setSearch(e.target.value)} onKeyDown={e => e.key === 'Escape' && setSearchOpen(false)} />
                <button className="linkButton" onClick={() => { setSearchOpen(false); setSearch(''); }}><X size={16} /></button>
              </div>
            ) : (
              <button className="linkButton" onClick={() => setSearchOpen(true)}><Search size={18} /><span className="searchPlaceholder">Search...</span></button>
            )}
          </div>
          <div className="userbar"><Bell size={20} /><span>{user?.FullName || user?.full_name || 'Owner'}</span><button onClick={logout} title="Log out"><LogOut size={18} /></button></div>
        </header>
        {React.Children.map(children, child => {
          if (React.isValidElement(child)) {
            return React.cloneElement(child, { searchQuery: search });
          }
          return child;
        })}
      </section>
    </div>
  );
}

function usePaginatedData(items, pageSize = 20) {
  const [page, setPage] = useState(1);
  const totalPages = Math.max(1, Math.ceil(items.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const start = (currentPage - 1) * pageSize;
  const paginated = items.slice(start, start + pageSize);
  return { items: paginated, page: currentPage, totalPages, setPage };
}

function Pagination({ page, totalPages, setPage }) {
  if (totalPages <= 1) return null;
  return (
    <div className="pagination">
      <button className="btn ghost" disabled={page <= 1} onClick={() => setPage(page - 1)}>Previous</button>
      <span>Page {page} of {totalPages}</span>
      <button className="btn ghost" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>Next</button>
    </div>
  );
}

function Dashboard() {
  const [data, setData] = useState(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    request('/dashboard').then(d => { setData(d); setError(''); }).catch(e => setError(e.message)).finally(() => setLoading(false));
  }, []);
  if (loading) return <Page title="Dashboard"><Skeleton count={4} /></Page>;
  if (error) return <Page title="Dashboard"><p className="error">{error}</p></Page>;
  if (!data) return <Page title="Dashboard"><p>Loading live portfolio...</p></Page>;
  const pct = data.total_due_cents ? Math.round((data.collected_cents / data.total_due_cents) * 100) : 0;
  return (
    <Page title="Collections" subtitle="Live values from your ledger for the current month.">
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

function Skeleton({ count = 3 }) {
  return <div className="skeleton">{Array.from({ length: count }).map((_, i) => <div key={i} className="skeletonLine" />)}</div>;
}

function Tenants({ searchQuery = '' }) {
  const [tenants, setTenants] = useState([]);
  const [properties, setProperties] = useState([]);
  const [units, setUnits] = useState([]);
  const [form, setForm] = useState({ full_name: '', phone: '', email: '' });
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [importOpen, setImportOpen] = useState(false);
  const [editing, setEditing] = useState(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(() => {
    setError('');
    return Promise.all([
      request('/tenants'),
      request('/properties'),
      request('/units')
    ]).then(([tenantData, propertyData, unitData]) => {
      setTenants(Array.isArray(tenantData) ? tenantData : []);
      setProperties(Array.isArray(propertyData) ? propertyData : []);
      setUnits(Array.isArray(unitData) ? unitData : []);
    }).catch(e => setError(e.message)).finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  async function add(e) {
    e.preventDefault();
    setError('');
    await request('/tenants', { method: 'POST', body: JSON.stringify(form) }).then(() => { setForm({ full_name: '', phone: '', email: '' }); load(); }).catch(e => setError(e.message));
  }

  async function copyTenantLink(tenantID) {
    setError('');
    const out = await request(`/tenants/${tenantID}/access-link`, { method: 'POST', body: '{}' }).catch(e => setError(e.message));
    if (!out) return;
    try {
      await navigator.clipboard.writeText(out.url);
      setNotice('Tenant portal link copied. Send it by SMS or WhatsApp.');
    } catch {
      setNotice('Copy the link from the response: ' + out.url);
    }
  }

  function startEdit(tenant) {
    setEditing({
      id: tenant.id,
      full_name: tenant.full_name || '',
      phone: tenant.phone || '',
      email: tenant.email || '',
      property_id: tenant.property_id || '',
      unit_id: tenant.unit_id || ''
    });
  }

  async function saveEdit() {
    setError('');
    const selectedUnit = units.find(unit => unit.id === editing.unit_id);
    const out = await request(`/tenants/${editing.id}`, {
      method: 'PUT',
      body: JSON.stringify({
        full_name: editing.full_name,
        phone: editing.phone,
        email: editing.email,
        unit_id: editing.unit_id,
        rent_cents: selectedUnit?.monthly_rent_cents || 0
      })
    }).catch(e => setError(e.message));
    if (out) {
      setNotice('Tenant updated');
      setEditing(null);
      load();
    }
  }

  function unitsForProperty(propertyID) {
    return units.filter(unit => unit.property_id === propertyID);
  }

  const filtered = tenants.filter(t => {
    if (!searchQuery) return true;
    const q = searchQuery.toLowerCase();
    return (t.full_name || '').toLowerCase().includes(q) ||
           (t.phone || '').includes(q) ||
           (t.email || '').toLowerCase().includes(q) ||
           (t.unit_label || '').toLowerCase().includes(q) ||
           (t.property_name || '').toLowerCase().includes(q);
  });

  const { items: paginated, page, totalPages, setPage } = usePaginatedData(filtered);

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
      {loading ? <Skeleton count={5} /> : (
        <>
          <section className="panel tablePanel">
            {paginated.length === 0 ? <Empty title={searchQuery ? "No matching tenants" : "No tenants yet"} text={searchQuery ? "Try a different search term." : "Create or import tenants to populate your ledger."} /> : <table><thead><tr><th>Name</th><th>Phone</th><th>Email</th><th>Property</th><th>Unit</th><th>Rent</th><th>Tenant access</th><th>Actions</th></tr></thead><tbody>{paginated.map(t => {
              const isEditing = editing?.id === t.id;
              const rowPropertyID = isEditing ? editing.property_id : t.property_id;
              const rowUnitID = isEditing ? editing.unit_id : t.unit_id;
              const rowUnit = units.find(unit => unit.id === rowUnitID);
              return (
                <tr key={t.id}>
                  <td>{isEditing ? <input value={editing.full_name} onChange={e => setEditing({ ...editing, full_name: e.target.value })} /> : t.full_name}</td>
                  <td>{isEditing ? <input value={editing.phone} onChange={e => setEditing({ ...editing, phone: e.target.value })} /> : t.phone}</td>
                  <td>{isEditing ? <input value={editing.email} onChange={e => setEditing({ ...editing, email: e.target.value })} /> : t.email}</td>
                  <td>{isEditing ? <select value={editing.property_id} onChange={e => setEditing({ ...editing, property_id: e.target.value, unit_id: '' })}><option value="">Choose property</option>{properties.map(property => <option key={property.id} value={property.id}>{property.name}</option>)}</select> : t.property_name}</td>
                  <td>{isEditing ? <select value={editing.unit_id} disabled={!editing.property_id} onChange={e => setEditing({ ...editing, unit_id: e.target.value })}><option value="">Choose unit</option>{unitsForProperty(editing.property_id).map(unit => <option key={unit.id} value={unit.id}>{unit.label} · {kes(unit.monthly_rent_cents || 0)}</option>)}</select> : t.unit_label}</td>
                  <td>{kes((isEditing ? rowUnit?.monthly_rent_cents : t.rent_cents) || 0)}</td>
                  <td><button className="linkButton" onClick={() => copyTenantLink(t.id)}>Copy link</button></td>
                  <td>{isEditing ? <div className="rowActions"><button className="linkButton" onClick={saveEdit}>Save</button><button className="linkButton muted" onClick={() => setEditing(null)}>Cancel</button></div> : <button className="linkButton" onClick={() => startEdit(t)}>Edit</button>}</td>
                </tr>
              );
            })}</tbody></table>}
          </section>
          <Pagination page={page} totalPages={totalPages} setPage={setPage} />
        </>
      )}
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
  ['deposit_kes', 'Deposit KES', false],
  ['payment_method', 'Payment method', false],
  ['bank_name', 'Bank name', false],
  ['bank_account_number', 'Bank account number', false],
  ['mpesa_paybill', 'M-Pesa paybill', false],
  ['mpesa_account_number', 'M-Pesa account number', false]
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
  const [loading, setLoading] = useState(true);

  function load() {
    setError('');
    return request('/tenant/me', { token }).then(d => { setData(d); setError(''); }).catch(e => setError(e.message)).finally(() => setLoading(false));
  }

  useEffect(() => { load(); }, [token]);

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
          {loading ? <p>Loading...</p> : !data ? <p>No data available</p> : data.payments.length === 0 ? <Empty title="No open payments" text="There are no due or overdue rent items assigned to this portal link." /> : data.payments.map(payment => (
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

function Payments({ searchQuery = '' }) {
  const [payments, setPayments] = useState([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    request('/payments').then(data => { setPayments(Array.isArray(data) ? data : []); setError(''); }).catch(e => setError(e.message)).finally(() => setLoading(false));
  }, []);

  const filtered = payments.filter(p => {
    if (!searchQuery) return true;
    const q = searchQuery.toLowerCase();
    return (p.tenant_name || '').toLowerCase().includes(q) ||
           (p.tenant_phone || '').includes(q) ||
           (p.unit_label || '').toLowerCase().includes(q) ||
           (p.property_name || '').toLowerCase().includes(q) ||
           (p.transaction_ref || '').toLowerCase().includes(q) ||
           (p.status || '').toLowerCase().includes(q);
  });

  const { items: paginated, page, totalPages, setPage } = usePaginatedData(filtered);

  return (
    <Page title="Payments" subtitle="Track due, overdue, submitted, and verified rent payments from the live ledger.">
      {error && <p className="error">{error}</p>}
      {loading ? <Skeleton count={5} /> : (
        <>
          <section className="panel tablePanel">
            {paginated.length === 0 ? <Empty title={searchQuery ? "No matching payments" : "No payments yet"} text={searchQuery ? "Try a different search term." : "Payment items are created when active leases exist for the current month."} /> : (
              <table>
                <thead><tr><th>Tenant</th><th>Unit</th><th>Due on</th><th>Amount</th><th>Status</th><th>Reference</th><th>Evidence</th></tr></thead>
                <tbody>{paginated.map(payment => (
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
          <Pagination page={page} totalPages={totalPages} setPage={setPage} />
        </>
      )}
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
  const [loading, setLoading] = useState(true);
  useEffect(() => { request('/settings').then(d => { setForm(d); setError(''); }).catch(e => setError(e.message)).finally(() => setLoading(false)); }, []);
  async function save(e) {
    e.preventDefault();
    setNotice('');
    await request('/settings', { method: 'PUT', body: JSON.stringify(form) }).then(() => setNotice('Settings saved')).catch(e => setError(e.message));
  }
  async function reminders() {
    const out = await request('/communications/reminders/run', { method: 'POST', body: '{}' }).catch(e => setError(e.message));
    if (out) setNotice(`Reminder run complete: ${out.sent} sent, ${out.skipped} skipped, ${out.failed} failed`);
  }
  if (loading) return <Page title="Settings"><Skeleton count={3} /></Page>;
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
