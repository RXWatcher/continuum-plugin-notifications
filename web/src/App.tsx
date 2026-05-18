import { cloneElement, useEffect, useMemo, useState } from "react";
import type { ReactElement, ReactNode } from "react";
import {
  Activity,
  Bell,
  CheckCircle2,
  Edit3,
  Play,
  Plus,
  Radio,
  RefreshCw,
  Route,
  Save,
  Search,
  Send,
  Server,
  Trash2,
  Users,
  XCircle,
} from "lucide-react";
import { mountPath } from "./lib/mountPath";

type ProviderField = {
  key: string;
  label: string;
  secret?: boolean;
  required?: boolean;
  control?: "text" | "password" | "url" | "email" | "number" | "checkbox" | "options" | "color";
  placeholder?: string;
  help?: string;
  default?: string;
  options?: { value: string; label: string }[];
};
type Provider = { id: string; name: string; fields: ProviderField[] };
type ContinuumUser = { id: number; username: string; email: string; role: string; enabled: boolean };
type Target = { id?: string; name: string; provider: string; enabled: boolean; config: Record<string, string> };
type Rule = { id?: string; name: string; event_pattern: string; target_ids: string[]; enabled: boolean; title: string; body: string };
type Delivery = { id: string; event_name: string; provider: string; title: string; status: string; attempts: number; last_error?: string; created_at: string };
type Contact = { id?: string; user_id: string; kind: string; value: string; label: string; enabled: boolean; verified: boolean };
type Tab = "providers" | "targets" | "rules" | "contacts" | "audit";

let cachedToken: string | null = null;
let refreshPromise: Promise<string | null> | null = null;

function captureTokenFromURL() {
  const params = new URLSearchParams(window.location.search);
  const token = params.get("token");
  if (!token) return;
  cachedToken = token;
  params.delete("token");
  const clean = `${window.location.pathname}${params.toString() ? `?${params.toString()}` : ""}${window.location.hash}`;
  window.history.replaceState(null, "", clean);
}

captureTokenFromURL();

async function authHeaders(): Promise<Record<string, string>> {
  const token = cachedToken ?? (await refreshAccessToken());
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function refreshAccessToken(): Promise<string | null> {
  if (refreshPromise) return refreshPromise;
  refreshPromise = (async () => {
    let refreshToken: string | null = null;
    try {
      refreshToken = window.localStorage.getItem("refresh_token");
    } catch {
      return null;
    }
    if (!refreshToken) return null;

    const response = await fetch("/api/v1/auth/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refreshToken }),
      credentials: "include",
    });
    if (!response.ok) return null;
    const data = await response.json();
    cachedToken = data.access_token ?? null;
    if (data.refresh_token) {
      try {
        window.localStorage.setItem("refresh_token", data.refresh_token);
      } catch {
        // Storage may be unavailable; keep the in-memory access token.
      }
    }
    return cachedToken;
  })().finally(() => {
    refreshPromise = null;
  });
  return refreshPromise;
}

const api = (path: string, init?: RequestInit) =>
  authHeaders().then((headers) => fetch(`${mountPath()}/api/admin${path}`, {
      ...init,
      credentials: "include",
      headers: { "Content-Type": "application/json", ...headers, ...(init?.headers || {}) },
    })).then(async (r) => {
    const text = await r.text();
    const contentType = r.headers.get("content-type") || "";
    if (!contentType.includes("application/json")) {
      if (r.status === 401 || r.status === 403) {
        throw new Error("Sign in to Continuum as an admin, then reopen Notifications from the admin sidebar.");
      }
      throw new Error(`Expected JSON from ${r.url}, received: ${text.slice(0, 120)}`);
    }
    const data = text ? parseJSON(text, r.url) : null;
    if (!r.ok) throw new Error(data?.error?.message || data?.message || r.statusText);
    if (r.status === 204) return null;
    return data;
  });

const hostApi = (path: string) =>
  authHeaders().then((headers) => fetch(path, {
    credentials: "include",
    headers,
  })).then(async (r) => {
    const data = await r.json().catch(() => null);
    if (!r.ok) throw new Error(data?.error?.message || data?.message || r.statusText);
    return data;
  });

function parseJSON(text: string, url: string) {
  try {
    return JSON.parse(text);
  } catch {
    throw new Error(`Expected JSON from ${url}, received: ${text.slice(0, 120)}`);
  }
}

const emptyTarget = (provider = "webhook"): Target => ({ name: "", provider, enabled: true, config: {} });
const emptyRule = (): Rule => ({ name: "", event_pattern: "plugin.*", target_ids: [], enabled: true, title: "{{event}}", body: "{{summary}}" });
const emptyContact = (): Contact => ({ user_id: "", kind: "email", value: "", label: "", enabled: true, verified: false });

export default function App() {
  const [tab, setTab] = useState<Tab>("targets");
  const [query, setQuery] = useState("");
  const [providers, setProviders] = useState<Provider[]>([]);
  const [targets, setTargets] = useState<Target[]>([]);
  const [rules, setRules] = useState<Rule[]>([]);
  const [deliveries, setDeliveries] = useState<Delivery[]>([]);
  const [users, setUsers] = useState<ContinuumUser[]>([]);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [target, setTarget] = useState<Target>(emptyTarget());
  const [rule, setRule] = useState<Rule>(emptyRule());
  const [contact, setContact] = useState<Contact>(emptyContact());
  const [error, setError] = useState("");

  const selectedProvider = useMemo(() => providers.find((p) => p.id === target.provider), [providers, target.provider]);
  const filteredProviders = providers.filter((p) => matches(query, p.name, p.id));
  const filteredTargets = targets.filter((t) => matches(query, t.name, t.provider));
  const filteredRules = rules.filter((r) => matches(query, r.name, r.event_pattern));
  const filteredContacts = contacts.filter((c) => matches(query, c.user_id, c.kind, c.value, c.label));
  const filteredDeliveries = deliveries.filter((d) => matches(query, d.event_name, d.provider, d.status, d.last_error || ""));
  const delivered = deliveries.filter((d) => d.status === "delivered").length;
  const failed = deliveries.filter((d) => d.status === "failed").length;
  const queued = deliveries.filter((d) => d.status === "queued").length;

  const load = async () => {
    setError("");
    try {
      const [p, t, r, d, c, u] = await Promise.all([api("/providers"), api("/targets"), api("/rules"), api("/deliveries?limit=100"), api("/contacts"), hostApi("/api/v1/admin/users").catch(() => [])]);
      setProviders(p);
      setTargets(t);
      setRules(r);
      setDeliveries(d);
      setContacts(c || []);
      setUsers((Array.isArray(u) ? u : []).filter((user) => user.enabled && user.email));
      if (p[0] && !target.provider) setTarget((x) => ({ ...x, provider: p[0].id }));
    } catch (e) {
      setError(String(e));
    }
  };

  useEffect(() => { void load(); }, []);

  const saveTarget = async () => {
    await api(target.id ? `/targets/${target.id}` : "/targets", { method: target.id ? "PUT" : "POST", body: JSON.stringify(target) });
    setTarget(emptyTarget(providers[0]?.id || "webhook"));
    await load();
  };

  const saveRule = async () => {
    await api(rule.id ? `/rules/${rule.id}` : "/rules", { method: rule.id ? "PUT" : "POST", body: JSON.stringify(rule) });
    setRule(emptyRule());
    await load();
  };

  const saveContact = async () => {
    await api(contact.id ? `/contacts/${contact.id}` : "/contacts", { method: contact.id ? "PUT" : "POST", body: JSON.stringify(contact) });
    setContact(emptyContact());
    await load();
  };

  const syncEmailContacts = async () => {
    await Promise.all(users.map((u) => api("/contacts", { method: "POST", body: JSON.stringify({ user_id: String(u.id), kind: "email", value: u.email, label: u.username, enabled: true, verified: true }) })));
    await load();
  };

  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-border sticky top-0 z-20 border-b bg-background/95 backdrop-blur">
        <div className="mx-auto flex max-w-[1500px] items-center justify-between gap-4 px-5 py-3">
          <div className="flex min-w-0 items-center gap-3">
            <span className="bg-primary/10 text-primary grid size-10 place-items-center rounded-md"><Bell className="size-5" /></span>
            <div className="min-w-0">
              <h1 className="truncate text-lg font-semibold">Notifications Command Center</h1>
              <p className="text-muted-foreground text-xs">Provider catalog, routing rules, delivery audit, and retry control</p>
            </div>
          </div>
          <div className="flex shrink-0 gap-2">
            <button className="btn" onClick={() => api("/deliveries/run", { method: "POST" }).then(load)}><Play className="size-4" /> Run due</button>
            <button className="btn" onClick={load}><RefreshCw className="size-4" /> Refresh</button>
          </div>
        </div>
      </header>

      <main className="mx-auto grid max-w-[1500px] gap-5 p-5 xl:grid-cols-[260px_minmax(0,1fr)]">
        <aside className="space-y-4">
          <section className="surface-panel p-3">
            <nav className="space-y-1">
              <NavItem active={tab === "providers"} icon={<Server />} label="Providers" meta={providers.length} onClick={() => setTab("providers")} />
              <NavItem active={tab === "targets"} icon={<Radio />} label="Targets" meta={targets.length} onClick={() => setTab("targets")} />
              <NavItem active={tab === "rules"} icon={<Route />} label="Rules" meta={rules.length} onClick={() => setTab("rules")} />
              <NavItem active={tab === "contacts"} icon={<Users />} label="Contacts" meta={contacts.length} onClick={() => setTab("contacts")} />
              <NavItem active={tab === "audit"} icon={<Activity />} label="Audit" meta={deliveries.length} onClick={() => setTab("audit")} />
            </nav>
          </section>
          <section className="surface-panel p-4">
            <div className="space-y-3">
              <Metric label="Queued" value={queued} />
              <Metric label="Delivered" value={delivered} tone="success" />
              <Metric label="Failed" value={failed} tone="danger" />
            </div>
          </section>
        </aside>

        <section className="min-w-0 space-y-4">
          {error && <div className="border-destructive bg-destructive/5 text-destructive rounded-md border p-3 text-sm">{error}</div>}
          <div className="surface-panel p-4">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 className="text-xl font-semibold">{tabTitle(tab)}</h2>
                <p className="text-muted-foreground mt-1 text-sm">{tabSubtitle(tab)}</p>
              </div>
              <div className="flex gap-2">
                <div className="relative min-w-[280px]">
                  <Search className="text-muted-foreground pointer-events-none absolute left-3 top-2.5 size-4" />
                  <input className="input pl-9" placeholder="Search current view" value={query} onChange={(e) => setQuery(e.target.value)} />
                </div>
                {tab === "targets" && <button className="btn" onClick={() => setTarget(emptyTarget(providers[0]?.id || "webhook"))}><Plus className="size-4" /> New target</button>}
                {tab === "rules" && <button className="btn" onClick={() => setRule(emptyRule())}><Plus className="size-4" /> New rule</button>}
                {tab === "contacts" && <button className="btn" onClick={syncEmailContacts}><RefreshCw className="size-4" /> Sync emails</button>}
              </div>
            </div>
          </div>

          {tab === "providers" && <ProviderGrid providers={filteredProviders} />}
          {tab === "targets" && <><TargetEditor target={target} providers={providers} selectedProvider={selectedProvider} users={users} setTarget={setTarget} saveTarget={saveTarget} /><TargetsTable rows={filteredTargets} providers={providers} onEdit={setTarget} onDelete={(id) => api(`/targets/${id}`, { method: "DELETE" }).then(load)} onTest={(id) => api(`/targets/${id}/test`, { method: "POST" })} /></>}
          {tab === "rules" && <><RuleEditor rule={rule} targets={targets} setRule={setRule} saveRule={saveRule} /><RulesTable rows={filteredRules} targets={targets} onEdit={setRule} onDelete={(id) => api(`/rules/${id}`, { method: "DELETE" }).then(load)} /></>}
          {tab === "contacts" && <><ContactEditor contact={contact} users={users} setContact={setContact} saveContact={saveContact} /><ContactsTable rows={filteredContacts} users={users} onEdit={setContact} onDelete={(id) => api(`/contacts/${id}`, { method: "DELETE" }).then(load)} /></>}
          {tab === "audit" && <AuditTable rows={filteredDeliveries} />}
        </section>
      </main>
    </div>
  );
}

function ProviderGrid({ providers }: { providers: Provider[] }) {
  return <div className="grid gap-3 md:grid-cols-2 2xl:grid-cols-3">{providers.map((p) => <section key={p.id} className="surface-panel p-4"><div className="flex items-start justify-between gap-3"><div className="min-w-0"><h3 className="truncate text-sm font-semibold">{p.name}</h3><p className="text-muted-foreground mt-1 font-mono text-xs">{p.id}</p></div><Badge tone="success">Native</Badge></div><div className="mt-4 flex flex-wrap gap-1.5">{p.fields.slice(0, 5).map((f) => <span key={f.key} className="chip">{f.label}</span>)}{p.fields.length > 5 && <span className="chip">+{p.fields.length - 5}</span>}</div></section>)}</div>;
}

function TargetsTable({ rows, providers, onEdit, onDelete, onTest }: { rows: Target[]; providers: Provider[]; onEdit: (row: Target) => void; onDelete: (id: string) => void; onTest: (id: string) => void }) {
  return <DataPanel empty="No targets configured. Create a target above the table."><table className="data-table"><thead><tr><Head>Name</Head><Head>Provider</Head><Head>Status</Head><Head align="right">Actions</Head></tr></thead><tbody>{rows.map((row) => <tr key={row.id}><Cell strong>{row.name}</Cell><Cell>{providers.find((p) => p.id === row.provider)?.name || row.provider}</Cell><Cell><Badge tone={row.enabled ? "success" : "muted"}>{row.enabled ? "Enabled" : "Disabled"}</Badge></Cell><Cell align="right"><ActionGroup><button className="icon" title="Send test" onClick={() => row.id && onTest(row.id)}><Send className="size-4" /></button><button className="icon" title="Edit" onClick={() => onEdit(row)}><Edit3 className="size-4" /></button><button className="icon danger" title="Delete" onClick={() => row.id && onDelete(row.id)}><Trash2 className="size-4" /></button></ActionGroup></Cell></tr>)}</tbody></table>{rows.length === 0 && <Empty text="No targets configured. Create a target above the table." />}</DataPanel>;
}

function RulesTable({ rows, targets, onEdit, onDelete }: { rows: Rule[]; targets: Target[]; onEdit: (row: Rule) => void; onDelete: (id: string) => void }) {
  return <DataPanel empty="No rules configured."><table className="data-table"><thead><tr><Head>Name</Head><Head>Pattern</Head><Head>Targets</Head><Head>Status</Head><Head align="right">Actions</Head></tr></thead><tbody>{rows.map((row) => <tr key={row.id}><Cell strong>{row.name}</Cell><Cell mono>{row.event_pattern}</Cell><Cell>{row.target_ids.map((id) => targets.find((t) => t.id === id)?.name || id).join(", ")}</Cell><Cell><Badge tone={row.enabled ? "success" : "muted"}>{row.enabled ? "Enabled" : "Disabled"}</Badge></Cell><Cell align="right"><ActionGroup><button className="icon" title="Edit" onClick={() => onEdit(row)}><Edit3 className="size-4" /></button><button className="icon danger" title="Delete" onClick={() => row.id && onDelete(row.id)}><Trash2 className="size-4" /></button></ActionGroup></Cell></tr>)}</tbody></table>{rows.length === 0 && <Empty text="No rules configured." />}</DataPanel>;
}

function AuditTable({ rows }: { rows: Delivery[] }) {
  return <DataPanel empty="No deliveries yet."><table className="data-table"><thead><tr><Head>Event</Head><Head>Provider</Head><Head>Status</Head><Head>Attempts</Head><Head>Error</Head></tr></thead><tbody>{rows.map((row) => <tr key={row.id}><Cell mono>{row.event_name}</Cell><Cell>{row.provider}</Cell><Cell><Badge tone={row.status === "failed" ? "danger" : row.status === "delivered" ? "success" : "warning"}>{row.status}</Badge></Cell><Cell>{row.attempts}</Cell><Cell>{row.last_error || ""}</Cell></tr>)}</tbody></table>{rows.length === 0 && <Empty text="No deliveries yet." />}</DataPanel>;
}

function ContactsTable({ rows, users, onEdit, onDelete }: { rows: Contact[]; users: ContinuumUser[]; onEdit: (row: Contact) => void; onDelete: (id: string) => void }) {
  return <DataPanel empty="No contacts configured. Sync Continuum emails or create a contact above."><table className="data-table"><thead><tr><Head>User</Head><Head>Kind</Head><Head>Value</Head><Head>Status</Head><Head align="right">Actions</Head></tr></thead><tbody>{rows.map((row) => <tr key={row.id}><Cell strong>{users.find((u) => String(u.id) === row.user_id)?.username || row.user_id}</Cell><Cell>{row.kind}</Cell><Cell>{row.value}</Cell><Cell><Badge tone={row.enabled ? "success" : "muted"}>{row.enabled ? "Enabled" : "Disabled"}</Badge></Cell><Cell align="right"><ActionGroup><button className="icon" title="Edit" onClick={() => onEdit(row)}><Edit3 className="size-4" /></button><button className="icon danger" title="Delete" onClick={() => row.id && onDelete(row.id)}><Trash2 className="size-4" /></button></ActionGroup></Cell></tr>)}</tbody></table>{rows.length === 0 && <Empty text="No contacts configured. Sync Continuum emails or create a contact above." />}</DataPanel>;
}

function TargetEditor({ target, providers, selectedProvider, users, setTarget, saveTarget }: { target: Target; providers: Provider[]; selectedProvider?: Provider; users: ContinuumUser[]; setTarget: (t: Target) => void; saveTarget: () => void }) {
  return <Inspector title={target.id ? "Edit Target" : "Create Target"} icon={<Radio />}><div className="editor-grid"><Input label="Name" value={target.name} onChange={(v) => setTarget({ ...target, name: v })} /><ProviderPicker providers={providers} value={target.provider} onChange={(provider) => setTarget({ ...target, provider, config: providerDefaults(providers.find((p) => p.id === provider)) })} /></div><div className="form-grid provider-config-grid">{selectedProvider?.fields.map((f) => <FieldInput key={f.key} field={f} providerID={selectedProvider.id} users={users} value={target.config[f.key] ?? ""} onChange={(v) => setTarget({ ...target, config: { ...target.config, [f.key]: v } })} />)}</div><div className="editor-actions"><label className="checkline"><input type="checkbox" checked={target.enabled} onChange={(e) => setTarget({ ...target, enabled: e.target.checked })} /> Enabled</label><button className="primary compact" onClick={saveTarget}><Save className="size-4" /> Save target</button></div></Inspector>;
}

function ContactEditor({ contact, users, setContact, saveContact }: { contact: Contact; users: ContinuumUser[]; setContact: (c: Contact) => void; saveContact: () => void }) {
  return <Inspector title={contact.id ? "Edit Contact" : "Create Contact"} icon={<Users />}><div className="editor-grid"><label className="field"><span>User</span><div className="target-list">{users.length === 0 ? <div className="target-empty">No Continuum users available.</div> : users.map((u) => <label key={u.id} className="target-option"><input type="radio" checked={contact.user_id === String(u.id)} onChange={() => setContact({ ...contact, user_id: String(u.id), value: contact.kind === "email" ? u.email : contact.value, label: contact.label || u.username })} /><span>{u.username || u.email}</span><code>{u.email}</code></label>)}</div></label><div className="editor-grid"><Input label="Kind" value={contact.kind} onChange={(kind) => setContact({ ...contact, kind })} /><Input label="Value" value={contact.value} onChange={(value) => setContact({ ...contact, value })} /><Input label="Label" value={contact.label} onChange={(label) => setContact({ ...contact, label })} /></div></div><div className="editor-actions"><span className="option-grid"><label className="checkline"><input type="checkbox" checked={contact.enabled} onChange={(e) => setContact({ ...contact, enabled: e.target.checked })} /> Enabled</label><label className="checkline"><input type="checkbox" checked={contact.verified} onChange={(e) => setContact({ ...contact, verified: e.target.checked })} /> Verified</label></span><button className="primary compact" onClick={saveContact}><Save className="size-4" /> Save contact</button></div></Inspector>;
}

function providerDefaults(provider?: Provider): Record<string, string> {
  const config: Record<string, string> = {};
  for (const field of provider?.fields ?? []) {
    if (field.default) config[field.key] = field.default;
  }
  return config;
}

function ProviderPicker({ providers, value, onChange }: { providers: Provider[]; value: string; onChange: (provider: string) => void }) {
  const [filter, setFilter] = useState("");
  const selected = providers.find((p) => p.id === value);
  const groups = useMemo(() => groupProviders(providers, filter), [providers, filter]);
  return <div className="field"><span>Provider</span><div className="provider-picker"><div className="provider-selected"><span>{selected?.name || "Select a provider"}</span><code>{selected?.id || ""}</code></div><input className="input" placeholder="Search providers" value={filter} onChange={(e) => setFilter(e.target.value)} /><div className="provider-groups">{groups.map((group) => <details key={group.key} className="provider-group" open={group.open}><summary><span>{group.label}</span><code>{group.rows.length}</code></summary><div className="provider-list">{group.rows.map((p) => <button key={p.id} type="button" className={`provider-option ${p.id === value ? "active" : ""}`} onClick={() => onChange(p.id)}><span>{p.name}</span><code>{p.id}</code></button>)}</div></details>)}</div></div></div>;
}

type ProviderGroup = {
  key: string;
  label: string;
  rows: Provider[];
  open: boolean;
};

function groupProviders(providers: Provider[], filter: string): ProviderGroup[] {
  const q = filter.trim().toLowerCase();
  const filtered = q ? providers.filter((p) => matches(q, p.name, p.id)) : providers;
  const buckets = [
    { key: "popular", label: "Popular" },
    { key: "chat", label: "Chat and Team" },
    { key: "email", label: "Email" },
    { key: "sms", label: "SMS and Voice" },
    { key: "push", label: "Push and Devices" },
    { key: "webhook", label: "Webhooks and Automation" },
    { key: "infra", label: "Infrastructure" },
    { key: "social", label: "Social and Community" },
    { key: "other", label: "Other" },
  ] as const;
  const grouped = new Map<string, Provider[]>();
  for (const bucket of buckets) grouped.set(bucket.key, []);
  for (const provider of filtered) {
    const key = providerGroupKey(provider);
    (grouped.get(key) ?? grouped.get("other") ?? []).push(provider);
  }
  return buckets
    .map((bucket) => {
      const rows = (grouped.get(bucket.key) ?? []).slice().sort((a, b) => {
        const left = providerPopularityRank(a);
        const right = providerPopularityRank(b);
        if (left !== right) return left - right;
        return a.name.localeCompare(b.name) || a.id.localeCompare(b.id);
      });
      return {
        key: bucket.key,
        label: bucket.label,
        rows,
        open: bucket.key === "popular" || !!q,
      };
    })
    .filter((group) => group.rows.length > 0);
}

function providerGroupKey(provider: Provider): string {
  const id = provider.id.toLowerCase();
  const name = provider.name.toLowerCase();
  if (providerPopularityRank(provider) < 999) return "popular";
  if (["slack", "discord", "msteams", "teams", "workflows", "google_chat", "mattermost", "rocketchat", "telegram", "signal_api", "viber", "wechat", "wecombot", "webexteams", "guilded", "lark", "feishu", "dingtalk", "synology", "kodi", "xbmc", "ntfy", "gotify", "notifiarr"].includes(id)) return "chat";
  if (["smtp", "smtp2go", "sendgrid", "sendpulse", "postmark", "resend", "mailgun", "brevo", "office365", "messagebird", "exotel"].includes(id) || name.includes("mail")) return "email";
  if (["twilio", "vonage", "plivo", "smseagle", "smsmanager", "sinch", "pushplus", "msg91", "kavenegar", "clicksend", "bulkvs", "bulksms", "burstsms", "africas_talking", "d7networks", "seven", "voipms", "join", "pushover"].includes(id)) return "sms";
  if (["pushbullet", "pushsafer", "pushy", "pushdeer", "pushjet", "pushme", "simplepush", "notica", "wxpusher", "bark", "macosx", "windows", "gnome", "glib", "dbus", "lametric", "lametric_cloud", "techuluspush"].includes(id) || name.includes("bridge") || name.includes("push")) return "push";
  if (["webhook", "custom_json", "custom_xml", "custom_form", "apprise_api", "ifttt", "custom_json", "custom_xml", "custom_form", "zoom", "twist", "flock", "flock", "dot", "qq", "reddit", "revolt", "ryver", "sfr", "spike", "synology", "discord", "mattermost"].includes(id)) return "webhook";
  if (["mqtt", "rsyslog", "syslog", "growl", "parseplatform", "home_assistant", "join", "alarms", "emby", "jellyfin", "misskey", "mastodon", "twitter", "bluesky", "aprs", "dapnet", "chanify", "serverchan", "kumulos", "evolution", "enigma2"].includes(id) || name.includes("bridge") || name.includes("server") || name.includes("platform")) return "infra";
  if (["facebook", "messenger", "twitter", "bluesky", "mastodon", "misskey", "linkedin", "reddit", "telegram", "viber", "zulip", "line", "lark", "feishu"].includes(id)) return "social";
  return "other";
}

function providerPopularityRank(provider: Provider): number {
  const rank = new Map<string, number>([
    ["slack", 1],
    ["telegram", 2],
    ["discord", 3],
    ["msteams", 4],
    ["teams", 5],
    ["webhook", 6],
    ["ntfy", 7],
    ["gotify", 8],
    ["smtp", 9],
    ["smtp2go", 10],
    ["sendgrid", 11],
    ["pushover", 12],
    ["signal_api", 13],
    ["pushbullet", 14],
    ["bark", 15],
    ["custom_json", 16],
    ["custom_form", 17],
    ["apprise_api", 18],
    ["notifiarr", 19],
    ["whatsapp", 20],
    ["wechat", 21],
    ["viber", 22],
  ]);
  return rank.get(provider.id) ?? 999;
}

function RuleEditor({ rule, targets, setRule, saveRule }: { rule: Rule; targets: Target[]; setRule: (r: Rule) => void; saveRule: () => void }) {
  return <Inspector title={rule.id ? "Edit Rule" : "Create Rule"} icon={<Route />}><div className="editor-grid"><Input label="Name" value={rule.name} onChange={(v) => setRule({ ...rule, name: v })} /><Input label="Event pattern" value={rule.event_pattern} onChange={(v) => setRule({ ...rule, event_pattern: v })} /><Input label="Title template" value={rule.title} onChange={(v) => setRule({ ...rule, title: v })} /></div><div className="editor-grid"><label className="field">Body template<textarea className="input min-h-20" value={rule.body} onChange={(e) => setRule({ ...rule, body: e.target.value })} /></label><TargetPicker targets={targets} value={rule.target_ids} onChange={(target_ids) => setRule({ ...rule, target_ids })} /></div><div className="editor-actions"><label className="checkline"><input type="checkbox" checked={rule.enabled} onChange={(e) => setRule({ ...rule, enabled: e.target.checked })} /> Enabled</label><button className="primary compact" onClick={saveRule}><Save className="size-4" /> Save rule</button></div></Inspector>;
}

function TargetPicker({ targets, value, onChange }: { targets: Target[]; value: string[]; onChange: (ids: string[]) => void }) {
  const selected = new Set(value);
  const toggle = (id: string) => onChange(selected.has(id) ? value.filter((x) => x !== id) : [...value, id]);
  return <div className="field"><span>Targets</span><div className="target-list">{targets.length === 0 ? <div className="target-empty">Create a notification target first.</div> : targets.map((t) => <label key={t.id} className="target-option"><input type="checkbox" checked={!!t.id && selected.has(t.id)} onChange={() => t.id && toggle(t.id)} /><span>{t.name}</span><code>{t.provider}</code></label>)}</div></div>;
}

function NavItem({ active, icon, label, meta, onClick }: { active: boolean; icon: ReactElement<{ className?: string }>; label: string; meta: number; onClick: () => void }) {
  return <button className={`nav-item ${active ? "active" : ""}`} onClick={onClick}>{cloneElement(icon, { className: "size-4" })}<span>{label}</span><span className="nav-meta">{meta}</span></button>;
}

function Metric({ label, value, tone }: { label: string; value: number; tone?: "success" | "danger" }) {
  const Icon = tone === "danger" ? XCircle : CheckCircle2;
  return <div className="metric"><div className="flex items-center justify-between"><span>{label}</span>{tone && <Icon className={tone === "danger" ? "size-4 text-destructive" : "size-4 text-success"} />}</div><strong>{value}</strong></div>;
}

function Inspector({ title, icon, children }: { title: string; icon: ReactElement<{ className?: string }>; children: ReactNode }) {
  return <section className="surface-panel overflow-hidden"><div className="panel-head"><span className="text-primary">{cloneElement(icon, { className: "size-4" })}</span><h2>{title}</h2></div><div className="space-y-3 p-4">{children}</div></section>;
}

function Input({ label, value, onChange, type = "text" }: { label: string; value: string; onChange: (v: string) => void; type?: string }) {
  return <label className="field">{label}<input className="input" type={type} value={value} onChange={(e) => onChange(e.target.value)} /></label>;
}

function FieldInput({ field, providerID, users, value, onChange }: { field: ProviderField; providerID: string; users: ContinuumUser[]; value: string; onChange: (v: string) => void }) {
  const displayValue = value || field.default || "";
  const label = <span>{field.label}{field.required && <b className="required-dot">*</b>}</span>;
  if (field.key === "to" && isEmailProvider(providerID)) {
    return <EmailRecipientField label={label} field={field} users={users} value={value} onChange={onChange} />;
  }
  if (field.control === "checkbox") {
    return <label className="field field-check">{label}<span className="checkline"><input type="checkbox" checked={isTruthy(displayValue)} onChange={(e) => onChange(e.target.checked ? "true" : "false")} /> Enabled</span>{field.help && <small>{field.help}</small>}</label>;
  }
  if (field.control === "options" && field.options?.length) {
    return <label className="field">{label}<span className="option-grid">{field.options.map((option) => <button key={option.value} type="button" className={`option-pill ${displayValue === option.value ? "active" : ""}`} onClick={() => onChange(option.value)}>{option.label}</button>)}</span>{field.help && <small>{field.help}</small>}</label>;
  }
  if (field.control === "color") {
    return <label className="field">{label}<span className="color-row"><input className="color-input" type="color" value={displayValue || "#00aaff"} onChange={(e) => onChange(e.target.value)} /><input className="input" value={displayValue} placeholder={field.placeholder || field.default || "#00aaff"} onChange={(e) => onChange(e.target.value)} /></span>{field.help && <small>{field.help}</small>}</label>;
  }
  return <label className="field">{label}<input className="input" type={field.secret ? "password" : field.control || "text"} value={value} placeholder={field.placeholder || field.default || ""} onChange={(e) => onChange(e.target.value)} />{field.help && <small>{field.help}</small>}</label>;
}

function isTruthy(value: string) {
  return ["true", "1", "yes", "on"].includes(value.trim().toLowerCase());
}

function EmailRecipientField({ label, field, users, value, onChange }: { label: ReactNode; field: ProviderField; users: ContinuumUser[]; value: string; onChange: (v: string) => void }) {
  const emails = splitRecipients(value);
  const selected = new Set(emails.map((email) => email.toLowerCase()));
  const userEmails = users.map((user) => user.email.toLowerCase());
  const manual = emails.filter((email) => !userEmails.includes(email.toLowerCase())).join(", ");
  const toggle = (email: string) => {
    const next = new Set(selected);
    const key = email.toLowerCase();
    if (next.has(key)) next.delete(key);
    else next.add(key);
    const ordered = [
      ...users.filter((user) => next.has(user.email.toLowerCase())).map((user) => user.email),
      ...emails.filter((existing) => !userEmails.includes(existing.toLowerCase()) && next.has(existing.toLowerCase())),
    ];
    onChange(ordered.join(", "));
  };
  const setManual = (manualValue: string) => {
    const picked = users.filter((user) => selected.has(user.email.toLowerCase())).map((user) => user.email);
    onChange([...picked, ...splitRecipients(manualValue)].join(", "));
  };
  return <label className="field email-recipient-field">{label}<div className="user-email-list">{users.length === 0 ? <div className="target-empty">No Continuum users with email addresses are available.</div> : users.map((user) => <span key={user.id} className="user-email-option"><input type="checkbox" checked={selected.has(user.email.toLowerCase())} onChange={() => toggle(user.email)} /><span><strong>{user.username || user.email}</strong><code>{user.email}</code></span></span>)}</div><input className="input" value={manual} placeholder="Additional email addresses" onChange={(e) => setManual(e.target.value)} />{field.help && <small>{field.help}</small>}</label>;
}

function splitRecipients(raw: string) {
  return raw.split(/[,\n;]/).map((value) => value.trim()).filter(Boolean);
}

function isEmailProvider(providerID: string) {
  return new Set(["smtp", "smtp2go", "sendgrid", "sendpulse", "postmark", "resend", "mailgun", "brevo", "sparkpost", "ses", "office365"]).has(providerID);
}

function DataPanel({ children }: { children: ReactNode; empty: string }) {
  return <section className="surface-panel overflow-hidden">{children}</section>;
}

function Empty({ text }: { text: string }) {
  return <div className="text-muted-foreground flex min-h-56 items-center justify-center p-8 text-sm"><div className="rounded-md border border-dashed border-border px-5 py-4">{text}</div></div>;
}

function ActionGroup({ children }: { children: ReactNode }) {
  return <div className="flex justify-end gap-1">{children}</div>;
}

function Head({ children, align }: { children: ReactNode; align?: "right" }) {
  return <th className={align === "right" ? "text-right" : ""}>{children}</th>;
}

function Cell({ children, align, strong, mono }: { children: ReactNode; align?: "right"; strong?: boolean; mono?: boolean }) {
  return <td className={`${align === "right" ? "text-right" : ""} ${strong ? "font-medium" : ""} ${mono ? "font-mono text-xs" : ""}`}>{children}</td>;
}

function Badge({ children, tone = "muted" }: { children: ReactNode; tone?: "success" | "danger" | "warning" | "muted" }) {
  return <span className={`badge ${tone}`}>{children}</span>;
}

function matches(query: string, ...values: string[]) {
  const q = query.trim().toLowerCase();
  return !q || values.some((v) => v.toLowerCase().includes(q));
}

function tabTitle(tab: Tab) {
  return ({ providers: "Provider Catalog", targets: "Notification Targets", rules: "Event Routing Rules", contacts: "User Contacts", audit: "Delivery Audit" })[tab];
}

function tabSubtitle(tab: Tab) {
  return ({
    providers: "All native Go delivery providers available to rules and targets.",
    targets: "Configured destinations with credentials and provider-specific settings.",
    rules: "Event patterns mapped to one or more notification targets.",
    contacts: "Per-user addresses and handles available to the base app and plugins.",
    audit: "Recent queued, delivered, and failed notifications.",
  })[tab];
}
