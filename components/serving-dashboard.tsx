'use client';

import { useEffect, useMemo, useState } from 'react';
import { flushSync } from 'react-dom';
import {
  Activity,
  Boxes,
  CheckCircle2,
  CircleGauge,
  Cloud,
  Cpu,
  Download,
  FileJson,
  Radio,
  ServerCog,
  ShieldCheck,
  Sparkles,
  Zap,
} from 'lucide-react';
import {
  Area,
  AreaChart,
  CartesianGrid,
  ReferenceLine,
  XAxis,
  YAxis,
} from 'recharts';

import overloadReport from '@/bench/results/reference-fake-overload.json';
import steadyReport from '@/bench/results/reference-fake-steady.json';
import { Badge } from '@/components/ui/badge';
import { buttonVariants } from '@/components/ui/button';
import {
  type ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart';
import { cn } from '@/lib/utils';

type ScenarioKey = 'steady' | 'overload';

const scenarios = {
  steady: {
    label: 'Steady load',
    note: '32 requests · concurrency 4',
    decision: 'Admit the workload',
    detail:
      'Capacity matches demand, so every request completes and no work is shed.',
    status: 'Within SLO',
    report: steadyReport,
    reportUrl: '/data/reference-fake-steady.json',
  },
  overload: {
    label: 'Burst overload',
    note: '48 requests · concurrency 12',
    decision: 'Shed batch traffic',
    detail:
      'Interactive work keeps its reserved path. Batch requests fail fast instead of turning overload into a timeout cascade.',
    status: 'Policy active',
    report: overloadReport,
    reportUrl: '/data/reference-fake-overload.json',
  },
} satisfies Record<
  ScenarioKey,
  {
    label: string;
    note: string;
    decision: string;
    detail: string;
    status: string;
    report: typeof steadyReport | typeof overloadReport;
    reportUrl: string;
  }
>;

const chartConfig = {
  ttft: { label: 'Measured TTFT', color: 'var(--color-chart-1)' },
} satisfies ChartConfig;

const architecture = [
  { label: 'OpenAI client', detail: 'One streaming contract', icon: Boxes },
  {
    label: 'Go gateway',
    detail: 'Deadline · priority · shed',
    icon: ShieldCheck,
  },
  { label: 'Inference engine', detail: 'fake · llama.cpp · vLLM', icon: Cpu },
  { label: 'Evidence', detail: 'Prometheus · raw JSON', icon: CircleGauge },
];

function isScenarioKey(value: unknown): value is ScenarioKey {
  return value === 'steady' || value === 'overload';
}

function formatMetric(value: number, digits = 1) {
  return value.toFixed(digits);
}

export function ServingDashboard() {
  const [scenario, setScenario] = useState<ScenarioKey>('overload');
  const active = scenarios[scenario];
  const { report } = active;
  const batch = report.summary.by_priority.batch;
  const interactive = report.summary.by_priority.interactive;
  const batchShedPercent = batch.requests
    ? (100 * batch.shed) / batch.requests
    : 0;

  const chartData = useMemo(
    () =>
      report.samples
        .filter(
          (
            sample,
          ): sample is (typeof report.samples)[number] & { ttft_ms: number } =>
            typeof sample.ttft_ms === 'number',
        )
        .map((sample, index) => ({
          request: index + 1,
          ttft: sample.ttft_ms,
          priority: sample.priority,
        })),
    [report],
  );

  const maxLatency = useMemo(
    () => Math.max(300, ...chartData.map((point) => point.ttft ?? 0)),
    [chartData],
  );

  const metrics = [
    {
      label: 'TTFT p95',
      value: formatMetric(report.summary.ttft_ms.p95),
      unit: 'ms',
      evidence: `${report.summary.completed} completed`,
    },
    {
      label: 'Inter-chunk p95',
      value: formatMetric(report.summary.mean_inter_chunk_ms.p95, 2),
      unit: 'ms/chunk',
      evidence: 'SSE chunk timing',
    },
    {
      label: 'Interactive goodput',
      value: formatMetric(interactive.goodput_percent),
      unit: '%',
      evidence: `${interactive.completed} / ${interactive.requests} within 300ms`,
    },
    {
      label: 'Batch shed',
      value: formatMetric(batchShedPercent),
      unit: '%',
      evidence: batch.requests
        ? `${batch.shed} / ${batch.requests} rejected explicitly`
        : 'No batch traffic offered',
    },
  ];

  useEffect(() => {
    const context = document.modelContext;
    if (!context?.registerTool) return;
    const lifecycle = new AbortController();
    try {
      void Promise.resolve(
        context.registerTool(
          {
            name: 'set_benchmark_scenario',
            title: 'Set benchmark scenario',
            description:
              'Switch the visible SLOServe dashboard between the committed steady-load and overload benchmark runs.',
            inputSchema: {
              type: 'object',
              properties: {
                scenario: {
                  type: 'string',
                  enum: ['steady', 'overload'],
                  description: 'The committed reference run to display.',
                },
              },
              required: ['scenario'],
              additionalProperties: false,
            },
            annotations: { readOnlyHint: false, untrustedContentHint: false },
            execute(input: unknown) {
              if (
                typeof input !== 'object' ||
                input === null ||
                !('scenario' in input) ||
                !isScenarioKey(input.scenario)
              ) {
                throw new Error(
                  'scenario must be either "steady" or "overload"',
                );
              }
              const nextScenario = input.scenario;
              flushSync(() => setScenario(nextScenario));
              return {
                scenario: nextScenario,
                label: scenarios[nextScenario].label,
                source: 'committed fake-engine reference run',
              };
            },
          },
          { signal: lifecycle.signal },
        ),
      ).catch((error) => console.warn('WebMCP registration failed', error));
    } catch (error) {
      console.warn('WebMCP registration failed', error);
    }
    return () => lifecycle.abort();
  }, []);

  return (
    <main className="min-h-screen bg-background text-foreground">
      <header className="sticky top-0 z-20 border-b border-white/8 bg-background/90 backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-[1560px] items-center justify-between px-4 sm:px-7">
          <div className="flex items-center gap-3">
            <div className="grid size-9 place-items-center rounded-xl border border-primary/25 bg-primary/10 text-primary shadow-[0_0_30px_rgb(103_232_194/10%)]">
              <Radio className="size-[18px]" />
            </div>
            <div>
              <div className="flex items-baseline gap-2">
                <span className="text-[15px] font-semibold tracking-[-0.02em]">
                  SLOServe
                </span>
                <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                  lab
                </span>
              </div>
              <p className="hidden text-[11px] text-muted-foreground sm:block">
                Inference control plane &amp; benchmark lab
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <a
              href="#architecture"
              className={cn(
                buttonVariants({ variant: 'ghost', size: 'sm' }),
                'hidden sm:inline-flex',
              )}
            >
              Architecture
            </a>
            <a
              href={active.reportUrl}
              download
              className={cn(
                buttonVariants({ variant: 'outline', size: 'sm' }),
                'border-white/10 bg-white/[0.03]',
              )}
            >
              <Download data-icon="inline-start" />
              Raw run
            </a>
          </div>
        </div>
      </header>

      <div className="mx-auto max-w-[1560px] px-4 py-5 sm:px-7 sm:py-7">
        <section className="mb-6 flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
          <div>
            <div className="mb-2 flex items-center gap-2 font-mono text-[10px] uppercase tracking-[0.18em] text-primary">
              <Activity className="size-3.5" />
              Executed gateway run · deterministic engine
            </div>
            <h1 className="max-w-3xl text-2xl font-semibold leading-tight tracking-[-0.035em] sm:text-[34px]">
              Keep useful tokens flowing when traffic turns hostile.
            </h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
              A Go control plane turns latency SLOs into bounded queues,
              priority isolation, cancellation, and explicit load shedding.
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2 rounded-2xl border border-white/8 bg-card/60 p-1.5 shadow-[0_14px_50px_rgb(0_0_0/18%)]">
            {(Object.keys(scenarios) as ScenarioKey[]).map((key) => (
              <button
                key={key}
                type="button"
                onClick={() => setScenario(key)}
                aria-pressed={scenario === key}
                className={`rounded-xl px-3 py-2 text-left transition-all sm:px-4 ${
                  scenario === key
                    ? 'bg-white/[0.09] text-foreground shadow-sm ring-1 ring-white/10'
                    : 'text-muted-foreground hover:bg-white/[0.04] hover:text-foreground'
                }`}
              >
                <span className="block text-xs font-medium sm:text-sm">
                  {scenarios[key].label}
                </span>
                <span className="hidden font-mono text-[9px] text-muted-foreground sm:block">
                  {scenarios[key].note}
                </span>
              </button>
            ))}
          </div>
        </section>

        <section
          aria-label="Benchmark summary"
          className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"
        >
          {metrics.map((metric) => (
            <article key={metric.label} className="metric-card group">
              <div className="flex items-center justify-between">
                <p className="text-xs font-medium text-muted-foreground">
                  {metric.label}
                </p>
                <span className="flex items-center gap-1 font-mono text-[9px] uppercase tracking-[0.12em] text-emerald-300">
                  <CheckCircle2 className="size-3" /> measured
                </span>
              </div>
              <div className="mt-4 flex items-end gap-2">
                <strong className="font-mono text-[34px] font-medium leading-none tracking-[-0.06em]">
                  {metric.value}
                </strong>
                <span className="pb-1 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
                  {metric.unit}
                </span>
              </div>
              <div className="mt-4 h-px bg-gradient-to-r from-primary/45 via-primary/10 to-transparent" />
              <p className="mt-3 font-mono text-[9px] uppercase tracking-[0.12em] text-muted-foreground">
                {metric.evidence}
              </p>
            </article>
          ))}
        </section>

        <section className="mt-3 grid gap-3 xl:grid-cols-[minmax(0,1.65fr)_minmax(330px,0.75fr)]">
          <article className="panel min-w-0 overflow-hidden">
            <div className="flex flex-col gap-4 border-b border-white/8 px-4 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-5">
              <div>
                <div className="flex items-center gap-2">
                  <CircleGauge className="size-4 text-primary" />
                  <h2 className="text-sm font-semibold">
                    Measured time to first token
                  </h2>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  One point per completed request · 300ms interactive SLO
                </p>
              </div>
              <Badge
                variant="outline"
                className="h-7 border-emerald-400/20 bg-emerald-400/6 px-2.5 text-emerald-300"
              >
                <span className="size-1.5 rounded-full bg-emerald-300" />
                Committed reference run
              </Badge>
            </div>
            <div className="px-2 pb-2 pt-5 sm:px-4">
              <ChartContainer
                config={chartConfig}
                className="aspect-auto h-[290px] w-full"
                initialDimension={{ width: 850, height: 290 }}
              >
                <AreaChart
                  data={chartData}
                  margin={{ left: 4, right: 16, top: 10, bottom: 4 }}
                >
                  <defs>
                    <linearGradient id="ttftFill" x1="0" y1="0" x2="0" y2="1">
                      <stop
                        offset="0%"
                        stopColor="var(--color-ttft)"
                        stopOpacity={0.26}
                      />
                      <stop
                        offset="100%"
                        stopColor="var(--color-ttft)"
                        stopOpacity={0}
                      />
                    </linearGradient>
                  </defs>
                  <CartesianGrid
                    vertical={false}
                    strokeDasharray="3 7"
                    stroke="rgba(255,255,255,.08)"
                  />
                  <XAxis
                    dataKey="request"
                    tickLine={false}
                    axisLine={false}
                    tickMargin={10}
                  />
                  <YAxis
                    domain={[0, Math.ceil(maxLatency / 100) * 100]}
                    tickLine={false}
                    axisLine={false}
                    tickMargin={8}
                    width={38}
                  />
                  <ReferenceLine
                    y={300}
                    stroke="var(--amber)"
                    strokeDasharray="4 5"
                    label={{
                      value: '300ms SLO',
                      position: 'insideTopRight',
                      fill: 'var(--muted-foreground)',
                      fontSize: 10,
                    }}
                  />
                  <ChartTooltip
                    cursor={false}
                    content={<ChartTooltipContent indicator="line" />}
                  />
                  <Area
                    type="monotone"
                    dataKey="ttft"
                    stroke="var(--color-ttft)"
                    strokeWidth={2.5}
                    fill="url(#ttftFill)"
                    dot={false}
                  />
                </AreaChart>
              </ChartContainer>
            </div>
          </article>

          <aside className="panel flex min-h-[390px] flex-col p-5 sm:p-6">
            <div className="flex items-center justify-between">
              <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                Control decision
              </p>
              <Badge
                variant="outline"
                className="border-primary/20 bg-primary/8 text-primary"
              >
                <Zap /> {active.status}
              </Badge>
            </div>
            <div className="mt-8 grid size-12 place-items-center rounded-2xl border border-amber-300/20 bg-amber-300/8 text-amber-200">
              <ShieldCheck className="size-6" />
            </div>
            <h2 className="mt-5 text-2xl font-semibold tracking-[-0.035em]">
              {active.decision}
            </h2>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              {active.detail}
            </p>
            <div className="my-6 h-px bg-white/8" />
            <dl className="space-y-4 text-xs">
              <div className="flex items-center justify-between">
                <dt className="flex items-center gap-2 text-muted-foreground">
                  <Cpu className="size-3.5" /> Running capacity
                </dt>
                <dd className="font-mono">4 sequences</dd>
              </div>
              <div className="flex items-center justify-between">
                <dt className="flex items-center gap-2 text-muted-foreground">
                  <ServerCog className="size-3.5" /> Interactive reserve
                </dt>
                <dd className="font-mono">1 slot</dd>
              </div>
              <div className="flex items-center justify-between">
                <dt className="flex items-center gap-2 text-muted-foreground">
                  <Cloud className="size-3.5" /> Queue budget
                </dt>
                <dd className="font-mono">4,000 tokens</dd>
              </div>
            </dl>
            <div className="mt-auto rounded-2xl border border-emerald-300/12 bg-emerald-300/[0.045] p-3.5">
              <div className="flex gap-2.5">
                <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-emerald-300" />
                <div>
                  <p className="text-xs font-medium text-emerald-100">
                    {report.summary.errors === 0
                      ? 'No unexpected errors'
                      : `${report.summary.errors} unexpected errors`}
                  </p>
                  <p className="mt-1 text-[11px] leading-5 text-emerald-100/55">
                    {report.summary.completed} completed, {report.summary.shed}{' '}
                    shed,{' '}
                    {formatMetric(report.summary.request_throughput_rps, 1)}{' '}
                    completed req/s.
                  </p>
                </div>
              </div>
            </div>
          </aside>
        </section>

        <section
          id="architecture"
          className="mt-3 grid gap-3 lg:grid-cols-[1.2fr_0.8fr]"
        >
          <article className="panel p-5 sm:p-6">
            <div className="flex items-center gap-2">
              <Sparkles className="size-4 text-primary" />
              <h2 className="text-sm font-semibold">
                One portable serving path
              </h2>
            </div>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
              The gateway owns reliability policy; the engine stays replaceable.
              The same client contract can target CI, a local model, or a
              private GPU service.
            </p>
            <div className="mt-6 grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
              {architecture.map((step, index) => (
                <div
                  key={step.label}
                  className="relative rounded-2xl border border-white/8 bg-white/[0.025] p-4"
                >
                  <step.icon className="size-4 text-primary" />
                  <p className="mt-7 text-sm font-medium">{step.label}</p>
                  <p className="mt-1 font-mono text-[9px] uppercase tracking-[0.11em] text-muted-foreground">
                    {step.detail}
                  </p>
                  {index < architecture.length - 1 && (
                    <span className="absolute -right-1.5 top-6 hidden size-3 rounded-full border border-primary/30 bg-background xl:block" />
                  )}
                </div>
              ))}
            </div>
          </article>

          <article className="panel p-5 sm:p-6">
            <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-primary">
              Research verdict
            </p>
            <h2 className="mt-3 text-xl font-semibold tracking-[-0.03em]">
              Build the serving loop. Skip the buzzword pile.
            </h2>
            <p className="mt-3 text-sm leading-6 text-muted-foreground">
              Local JobScan data found 177 serving roles across 51 companies.
              vLLM appeared in 49.2% and Kubernetes in 45.2% of that slice,
              while distributed-training terms were sparse in recent postings.
            </p>
            <div className="mt-5 grid grid-cols-3 gap-2">
              {[
                ['177', 'serving JDs'],
                ['51', 'companies'],
                ['+2.1pp', 'GPU platform'],
              ].map(([value, label]) => (
                <div
                  key={label}
                  className="rounded-xl border border-white/8 bg-white/[0.025] p-3"
                >
                  <strong className="font-mono text-lg font-medium text-foreground">
                    {value}
                  </strong>
                  <p className="mt-1 text-[10px] text-muted-foreground">
                    {label}
                  </p>
                </div>
              ))}
            </div>
          </article>
        </section>

        <footer className="mt-3 flex flex-col gap-3 rounded-2xl border border-white/8 bg-card/35 px-4 py-3 text-[11px] text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <span className="flex items-center gap-1.5">
              <FileJson className="size-3.5 text-primary" /> raw samples
              committed
            </span>
            <span>engine: fake-engine/v1</span>
            <span>generated: 2026-09-29</span>
          </div>
          <a
            href={active.reportUrl}
            className="font-medium text-foreground transition-colors hover:text-primary"
          >
            Inspect run manifest →
          </a>
        </footer>
      </div>
    </main>
  );
}
