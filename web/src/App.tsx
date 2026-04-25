import type { ReactNode } from "react";
import {
  Activity,
  Archive,
  Bell,
  CheckCircle2,
  Clock3,
  Database,
  HardDrive,
  KeyRound,
  Play,
  RotateCcw,
  Search,
  ShieldCheck,
  TriangleAlert,
} from "lucide-react";
import { Button } from "./components/ui/button";

const navItems = [
  { label: "Dashboard", icon: Activity, active: true },
  { label: "Targets", icon: Database },
  { label: "Backups", icon: Archive },
  { label: "Storage", icon: HardDrive },
  { label: "Keys", icon: KeyRound },
];

const jobs = [
  { target: "redis-prod", type: "incremental", status: "running", started: "14:03", bytes: "18.4 GiB" },
  { target: "postgres-ledger", type: "full", status: "verified", started: "13:41", bytes: "241 GiB" },
  { target: "mongo-events", type: "incremental", status: "queued", started: "14:15", bytes: "waiting" },
  { target: "mysql-core", type: "restore", status: "attention", started: "12:58", bytes: "77.2 GiB" },
];

const statusClass: Record<string, string> = {
  running: "bg-warning/15 text-warning",
  verified: "bg-success/15 text-success",
  queued: "bg-indigo/20 text-indigo-light",
  attention: "bg-danger/15 text-danger-light",
};

const metricTone = {
  success: "bg-success/15 text-success",
  warning: "bg-warning/15 text-warning",
  bronze: "bg-bronze/15 text-bronze",
  danger: "bg-danger/15 text-danger-light",
};

const healthTone = {
  success: "text-success",
  warning: "text-warning",
  bronze: "text-bronze",
  indigo: "text-indigo-light",
};

function AppLogo() {
  return (
    <div className="flex h-12 items-center gap-3 px-4">
      <img src="/kronos-mark.svg" className="h-8 w-8" alt="" />
      <div>
        <div className="font-wordmark text-lg font-black tracking-normal text-bronze">Kronos</div>
        <div className="text-xs text-muted">control plane</div>
      </div>
    </div>
  );
}

export function App() {
  return (
    <main className="min-h-screen bg-void text-marble">
      <div className="grid min-h-screen grid-cols-1 lg:grid-cols-[248px_1fr]">
        <aside className="border-line bg-panel/92 lg:border-r">
          <AppLogo />
          <nav className="grid gap-1 px-3 py-2">
            {navItems.map((item) => (
              <button
                key={item.label}
                className={`flex h-10 items-center gap-3 rounded-md px-3 text-sm font-medium transition ${
                  item.active ? "bg-bronze text-void" : "text-muted hover:bg-surface hover:text-marble"
                }`}
              >
                <item.icon className="h-4 w-4" />
                {item.label}
              </button>
            ))}
          </nav>
        </aside>

        <section className="min-w-0">
          <header className="flex min-h-16 items-center justify-between gap-3 border-b border-line px-4 sm:px-6">
            <div className="min-w-0">
              <h1 className="text-xl font-semibold text-marble">Operations</h1>
              <p className="text-sm text-muted">04/25/2026 · Europe/Tallinn</p>
            </div>
            <div className="flex items-center gap-2">
              <Button variant="ghost" title="Search" aria-label="Search" icon={<Search className="h-4 w-4" />} />
              <Button variant="ghost" title="Notifications" aria-label="Notifications" icon={<Bell className="h-4 w-4" />} />
              <Button variant="primary" icon={<Play className="h-4 w-4" />}>
                Run backup
              </Button>
            </div>
          </header>

          <div className="grid gap-6 px-4 py-6 sm:px-6 xl:grid-cols-[minmax(0,1fr)_340px]">
            <section className="grid gap-6">
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                <Metric icon={<CheckCircle2 />} label="Verified" value="128" tone="success" />
                <Metric icon={<Clock3 />} label="Queued" value="7" tone="warning" />
                <Metric icon={<ShieldCheck />} label="Protected" value="42" tone="bronze" />
                <Metric icon={<TriangleAlert />} label="Attention" value="2" tone="danger" />
              </div>

              <section className="overflow-hidden rounded-md border border-line bg-panel">
                <div className="flex items-center justify-between gap-3 border-b border-line px-4 py-3">
                  <h2 className="text-base font-semibold">Recent jobs</h2>
                  <Button variant="secondary" icon={<RotateCcw className="h-4 w-4" />}>
                    Refresh
                  </Button>
                </div>
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[680px] border-collapse text-left text-sm">
                    <thead className="bg-surface text-xs uppercase text-muted">
                      <tr>
                        <th className="px-4 py-3 font-semibold">Target</th>
                        <th className="px-4 py-3 font-semibold">Type</th>
                        <th className="px-4 py-3 font-semibold">Started</th>
                        <th className="px-4 py-3 font-semibold">Bytes</th>
                        <th className="px-4 py-3 font-semibold">Status</th>
                      </tr>
                    </thead>
                    <tbody>
                      {jobs.map((job) => (
                        <tr key={`${job.target}-${job.type}`} className="border-t border-line">
                          <td className="px-4 py-3 font-medium text-marble">{job.target}</td>
                          <td className="px-4 py-3 text-muted">{job.type}</td>
                          <td className="px-4 py-3 font-mono text-muted">{job.started}</td>
                          <td className="px-4 py-3 text-muted">{job.bytes}</td>
                          <td className="px-4 py-3">
                            <span className={`inline-flex h-7 items-center rounded-md px-2 text-xs font-semibold ${statusClass[job.status]}`}>
                              {job.status}
                            </span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
            </section>

            <aside className="grid content-start gap-6">
              <section className="rounded-md border border-line bg-panel p-4">
                <h2 className="text-base font-semibold">Repository health</h2>
                <div className="mt-4 grid gap-3">
                  <HealthRow label="Hash chain" value="valid" tone="success" />
                  <HealthRow label="Dedup ratio" value="6.8x" tone="bronze" />
                  <HealthRow label="Storage used" value="1.42 TiB" tone="indigo" />
                  <HealthRow label="Oldest restore point" value="42 days" tone="warning" />
                </div>
              </section>

              <section className="rounded-md border border-line bg-panel p-4">
                <h2 className="text-base font-semibold">Next runs</h2>
                <div className="mt-4 grid gap-3">
                  {["14:15 mongo-events", "14:30 redis-prod", "15:00 postgres-ledger"].map((item) => (
                    <div key={item} className="flex h-10 items-center justify-between rounded-md bg-surface px-3 text-sm">
                      <span>{item}</span>
                      <Clock3 className="h-4 w-4 text-bronze" />
                    </div>
                  ))}
                </div>
              </section>
            </aside>
          </div>
        </section>
      </div>
    </main>
  );
}

function Metric({
  icon,
  label,
  value,
  tone,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  tone: keyof typeof metricTone;
}) {
  return (
    <section className="rounded-md border border-line bg-panel p-4">
      <div className={`mb-5 inline-flex h-9 w-9 items-center justify-center rounded-md ${metricTone[tone]}`}>{icon}</div>
      <div className="text-2xl font-semibold">{value}</div>
      <div className="mt-1 text-sm text-muted">{label}</div>
    </section>
  );
}

function HealthRow({ label, value, tone }: { label: string; value: string; tone: keyof typeof healthTone }) {
  return (
    <div className="flex h-10 items-center justify-between gap-4 rounded-md bg-surface px-3 text-sm">
      <span className="text-muted">{label}</span>
      <span className={`font-semibold ${healthTone[tone]}`}>{value}</span>
    </div>
  );
}
