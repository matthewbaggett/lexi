#!/usr/bin/env node
// The equivalence suite: run each case in the real alex and in lexi, in scratch directories, and compare
// stdout, stderr, the exit code and the files that each tool leaves behind. See docs/Equivalence.md.
//
// Usage: node scripts/equivalence/run.mjs --lexi dist/lexi --alex testdata/oracle [options]
//   --filter <glob>   run the cases whose id matches (`*` is any text). Repeat the option to add patterns.
//   --row <id>        run the cases of one matrix row. Repeat the option to add rows.
//   --seed <n>        seed of the fuzz generator and of the corpus samples (EQUIV_SEED, default 1)
//   --fuzz <n>        number of fuzz cases (EQUIV_FUZZ, default 400)
//   --corpus <n>      corpus cases per sureness level and for the options (EQUIV_CORPUS, default 40)
//   --jobs <n>        cases at once (EQUIV_JOBS, default the number of CPUs)
//   --out <dir>       write the report and each mismatch here (default dist/equivalence)
//   --list            print the ids of the cases and stop
//   --keep            keep the scratch directories
// Exit: 0 when every case matches or is a known gap, 1 on a mismatch that known-gaps.json does not list.

import {spawn} from 'node:child_process'
import crypto from 'node:crypto'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import {fileURLToPath} from 'node:url'
import {allCases} from './cases.mjs'
import {mulberry32} from './fuzz.mjs'

const here = path.dirname(fileURLToPath(import.meta.url))
const repo = path.resolve(here, '..', '..')

// ---------------------------------------------------------------------------------------------------------
// Options

export function parseOptions(argv, env = process.env) {
  const o = {
    lexi: path.join(repo, 'dist', 'lexi'),
    alex: path.join(repo, 'testdata', 'oracle'),
    filter: [],
    row: [],
    seed: Number(env.EQUIV_SEED ?? 1),
    fuzz: Number(env.EQUIV_FUZZ ?? 400),
    corpus: Number(env.EQUIV_CORPUS ?? 40),
    jobs: Number(env.EQUIV_JOBS ?? os.cpus().length),
    out: path.join(repo, 'dist', 'equivalence'),
    list: false,
    keep: false
  }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    const value = () => {
      if (i + 1 >= argv.length) throw new Error(`${a} needs a value`)
      return argv[++i]
    }
    if (a === '--lexi') o.lexi = path.resolve(value())
    else if (a === '--alex') o.alex = path.resolve(value())
    else if (a === '--filter') o.filter.push(value())
    else if (a === '--row') o.row.push(value())
    else if (a === '--seed') o.seed = Number(value())
    else if (a === '--fuzz') o.fuzz = Number(value())
    else if (a === '--corpus') o.corpus = Number(value())
    else if (a === '--jobs') o.jobs = Number(value())
    else if (a === '--out') o.out = path.resolve(value())
    else if (a === '--list') o.list = true
    else if (a === '--keep') o.keep = true
    else throw new Error(`unknown option ${a}`)
  }
  for (const k of ['seed', 'fuzz', 'corpus', 'jobs']) {
    if (!Number.isInteger(o[k]) || o[k] < 0) throw new Error(`${k} must be a whole number`)
  }
  return o
}

export function globToRegExp(glob) {
  return new RegExp('^' + glob.split('*').map((s) => s.replace(/[.+?^${}()|[\]\\]/g, '\\$&')).join('.*') + '$')
}

// ---------------------------------------------------------------------------------------------------------
// Running a program in a scratch directory

/** The environment of a run: nothing from the caller, so a CI variable cannot change a result. */
function baseEnv(home) {
  return {
    PATH: process.env.PATH,
    HOME: home,
    TMPDIR: home,
    LANG: 'C.UTF-8',
    TERM: 'xterm',
    NO_UPDATE_NOTIFIER: '1',
    NODE_NO_WARNINGS: '1',
    GIT_CONFIG_GLOBAL: '/dev/null',
    GIT_CONFIG_NOSYSTEM: '1',
    GIT_AUTHOR_NAME: 'Equivalence',
    GIT_AUTHOR_EMAIL: 'equivalence@example.test',
    GIT_COMMITTER_NAME: 'Equivalence',
    GIT_COMMITTER_EMAIL: 'equivalence@example.test'
  }
}

function exec(cmd, args, {cwd, env, stdin}) {
  return new Promise((resolve) => {
    const child = spawn(cmd, args, {cwd, env, stdio: ['pipe', 'pipe', 'pipe']})
    const out = []
    const err = []
    const timer = setTimeout(() => child.kill('SIGKILL'), 120_000)
    child.stdout.on('data', (d) => out.push(d))
    child.stderr.on('data', (d) => err.push(d))
    child.stdin.on('error', () => {})
    child.stdin.end(stdin ?? '')
    child.on('error', (e) => {
      clearTimeout(timer)
      resolve({code: -1, signal: null, stdout: '', stderr: `spawn failed: ${e.message}`})
    })
    child.on('close', (code, signal) => {
      clearTimeout(timer)
      resolve({
        code: code ?? -1,
        signal,
        stdout: Buffer.concat(out).toString('latin1'),
        stderr: Buffer.concat(err).toString('latin1')
      })
    })
  })
}

/** A snapshot of the files below a directory, without .git: path to a digest and the executable bit. */
function snapshot(dir) {
  const out = {}
  const walk = (rel) => {
    for (const e of fs.readdirSync(path.join(dir, rel), {withFileTypes: true})) {
      const next = rel ? `${rel}/${e.name}` : e.name
      if (e.name === '.git') continue
      if (e.isDirectory()) walk(next)
      else {
        const full = path.join(dir, next)
        const st = fs.lstatSync(full)
        const kind = st.isSymbolicLink() ? 'link ' + fs.readlinkSync(full) : crypto.createHash('sha256').update(fs.readFileSync(full)).digest('hex').slice(0, 16)
        out[next] = `${kind}${st.mode & 0o111 ? ' x' : ''}`
      }
    }
  }
  walk('')
  return out
}

function treeChanges(before, after) {
  const names = new Set([...Object.keys(before), ...Object.keys(after)])
  const changes = []
  for (const n of [...names].sort()) {
    if (before[n] === undefined) changes.push(`created ${n} ${after[n]}`)
    else if (after[n] === undefined) changes.push(`removed ${n}`)
    else if (before[n] !== after[n]) changes.push(`changed ${n} ${after[n]}`)
  }
  return changes.join('\n')
}

/** Names of the JSON reporter that lexi knows. alex loads a reporter from the working directory, and the scratch directory has no node_modules, so alex gets the entry file. */
const JSON_REPORTER = /^(?:json|vfile-reporter-json|node_modules\/vfile-reporter-json(?:\/index\.js)?)$/

function alexArgs(args, alexDir) {
  const entry = path.join(alexDir, 'node_modules', 'vfile-reporter-json', 'index.js')
  return args.map((a, i) => {
    if (a.startsWith('--reporter=') && JSON_REPORTER.test(a.slice(11))) return `--reporter=${entry}`
    return args[i - 1] === '--reporter' && JSON_REPORTER.test(a) ? entry : a
  })
}

/** Run one case in one tool, in `<root>/w`. */
export async function runTool(tool, c, root, o) {
  const dir = path.join(root, 'w')
  fs.mkdirSync(dir, {recursive: true})
  for (const [name, content] of Object.entries(c.files)) {
    const file = path.join(dir, name)
    fs.mkdirSync(path.dirname(file), {recursive: true})
    fs.writeFileSync(file, content)
  }
  const env = {...baseEnv(root), ...c.env}
  if (c.setup) {
    const r = await exec('sh', ['-e', '-c', c.setup], {cwd: dir, env})
    if (r.code !== 0) throw new Error(`setup of ${c.id} failed: ${r.stderr}`)
  }
  const before = snapshot(dir)
  const argv = tool === 'alex' ? [process.execPath, path.join(o.alex, 'node_modules', 'alex', 'cli.js'), ...alexArgs(c.args, o.alex)] : [o.lexi, ...c.args]
  const full = c.tty ? ['python3', path.join(here, 'pty-run.py'), ...argv] : argv
  const r = await exec(full[0], full.slice(1), {cwd: dir, env, stdin: c.stdin})
  r.files = treeChanges(before, snapshot(dir))
  r.dir = fs.realpathSync(dir)
  r.root = fs.realpathSync(root)
  return r
}

/** Replace what must differ: the scratch paths and the program name. */
export function normalise(r) {
  const fix = (s) =>
    s
      .split(r.dir).join('<cwd>')
      .split(r.root).join('<tmp>')
      .replace(/\b(?:alex|lexi)\b/g, '<prog>')
  return {code: String(r.code) + (r.signal ? ` ${r.signal}` : ''), stdout: fix(r.stdout), stderr: fix(r.stderr), files: fix(r.files)}
}

const FIELDS = ['code', 'stdout', 'stderr', 'files']

export function differences(a, b) {
  return FIELDS.filter((f) => a[f] !== b[f])
}

// ---------------------------------------------------------------------------------------------------------
// Known gaps and the summary

/** Read known-gaps.json, and check each entry against docs/Parity.md. */
export function loadGaps(file, parityText, rowIds) {
  const gaps = JSON.parse(fs.readFileSync(file, 'utf8')).gaps
  const cells = new Set(
    parityText.split('\n').filter((l) => l.startsWith('|')).map((l) => l.split('|')[1]?.trim())
  )
  for (const g of gaps) {
    for (const f of g.fields ?? []) if (!FIELDS.includes(f)) throw new Error(`known gap ${g.id}: unknown field ${f}`)
    for (const k of ['id', 'parity', 'reason', 'cases']) if (!g[k] || g[k].length === 0) throw new Error(`known gap ${g.id ?? '?'} has no ${k}`)
    if (!cells.has(g.parity)) throw new Error(`known gap ${g.id}: docs/Parity.md has no row "${g.parity}"`)
    for (const p of g.cases) {
      if (!rowIds.has(p.split('/')[0])) throw new Error(`known gap ${g.id}: ${p} names no matrix row`)
    }
    g.regexps = g.cases.map(globToRegExp)
  }
  return gaps
}

/** The gap that covers a mismatch: it names the case, and every field that differs is a field it covers. */
export function gapFor(gaps, id, fields) {
  return gaps.find((g) => g.regexps.some((re) => re.test(id)) && fields.every((f) => (g.fields ?? FIELDS).includes(f)))
}

function table(rows, header) {
  const all = [header, ...rows]
  const width = header.map((_, i) => Math.max(...all.map((r) => String(r[i]).length)))
  const line = (r) => r.map((c, i) => (i === 0 ? String(c).padEnd(width[i]) : String(c).padStart(width[i]))).join('  ')
  return [line(header), width.map((w) => '-'.repeat(w)).join('  '), ...rows.map(line)].join('\n')
}

function clip(text, lines = 25) {
  const all = JSON.stringify(text).length > 6000 ? text.slice(0, 3000) + '\n… (cut)' : text
  const rows = all.split('\n')
  return (rows.length > lines ? [...rows.slice(0, lines), '… (cut)'] : rows).map((l) => '    ' + JSON.stringify(l).slice(1, -1)).join('\n')
}

// ---------------------------------------------------------------------------------------------------------
// Main

async function pool(items, jobs, work) {
  let next = 0
  const results = new Array(items.length)
  const worker = async () => {
    while (next < items.length) {
      const i = next++
      results[i] = await work(items[i], i)
    }
  }
  await Promise.all(Array.from({length: Math.max(1, jobs)}, worker))
  return results
}

async function main() {
  const o = parseOptions(process.argv.slice(2))
  const matrix = JSON.parse(fs.readFileSync(path.join(repo, 'testdata', 'equivalence', 'matrix.json'), 'utf8')).rows
  const rowIds = new Set(matrix.map((r) => r.id))
  let cases = allCases(repo, {mulberry32, seed: o.seed, fuzz: o.fuzz, corpus: o.corpus})
  for (const c of cases) if (!rowIds.has(c.row)) throw new Error(`case ${c.id} names the row ${c.row}, and matrix.json has no such row`)
  for (const r of matrix) if (!cases.some((c) => c.row === r.id)) throw new Error(`matrix row ${r.id} has no case`)
  const filters = o.filter.map(globToRegExp)
  cases = cases.filter((c) => (filters.length === 0 || filters.some((re) => re.test(c.id))) && (o.row.length === 0 || o.row.includes(c.row)))
  if (o.list) {
    for (const c of cases) console.log(c.id)
    return 0
  }
  const gaps = loadGaps(path.join(repo, 'testdata', 'equivalence', 'known-gaps.json'), fs.readFileSync(path.join(repo, 'docs', 'Parity.md'), 'utf8'), rowIds)
  const havePython = await exec('python3', ['--version'], {cwd: repo, env: baseEnv(os.tmpdir())}).then((r) => r.code === 0)

  const scratch = fs.mkdtempSync(path.join(os.tmpdir(), 'equiv-'))
  console.error(`equivalence: ${cases.length} cases, seed ${o.seed}, ${o.jobs} at once, scratch ${scratch}`)
  let done = 0
  const results = await pool(cases, o.jobs, async (c, i) => {
    if (c.tty && !havePython) return {c, status: 'skipped', diff: []}
    const base = path.join(scratch, String(i))
    try {
      const [a, l] = await Promise.all([runTool('alex', c, path.join(base, 'alex'), o), runTool('lexi', c, path.join(base, 'lexi'), o)])
      const na = normalise(a)
      const nl = normalise(l)
      const diff = differences(na, nl)
      if (++done % 100 === 0) console.error(`equivalence: ${done}/${cases.length}`)
      return {c, status: diff.length === 0 ? 'match' : 'mismatch', diff, na, nl}
    } catch (e) {
      return {c, status: 'mismatch', diff: ['error'], na: {error: String(e.message)}, nl: {}}
    }
  })
  if (!o.keep) fs.rmSync(scratch, {recursive: true, force: true})

  fs.rmSync(o.out, {recursive: true, force: true})
  fs.mkdirSync(o.out, {recursive: true})
  const usedGaps = new Set()
  const unexpected = []
  for (const r of results) {
    if (r.status !== 'mismatch') continue
    r.gap = gapFor(gaps, r.c.id, r.diff)
    if (r.gap) usedGaps.add(r.gap.id)
    else unexpected.push(r)
    const dir = path.join(o.out, 'mismatches', r.c.id)
    fs.mkdirSync(dir, {recursive: true})
    fs.writeFileSync(path.join(dir, 'case.json'), JSON.stringify({args: r.c.args, env: r.c.env, stdin: r.c.stdin?.toString(), files: Object.fromEntries(Object.entries(r.c.files).map(([k, v]) => [k, v.toString()])), setup: r.c.setup, gap: r.gap?.id}, null, 2))
    for (const [name, side] of [['alex', r.na], ['lexi', r.nl]]) {
      for (const f of FIELDS) fs.writeFileSync(path.join(dir, `${name}.${f}`), side[f] ?? side.error ?? '', 'latin1')
    }
  }

  const summary = matrix.map((row) => {
    const mine = results.filter((r) => r.c.row === row.id)
    const count = (s) => mine.filter((r) => r.status === s && (s !== 'mismatch' || !r.gap)).length
    return [row.id, mine.length, count('match'), mine.filter((r) => r.gap).length, count('mismatch'), count('skipped')]
  })
  const sum = (i) => summary.reduce((t, r) => t + r[i], 0)
  summary.push(['TOTAL', sum(1), sum(2), sum(3), sum(4), sum(5)])
  const header = ['row', 'cases', 'match', 'known gap', 'mismatch', 'skipped']
  console.log(table(summary.filter((r) => r[1] > 0 || r[0] === 'TOTAL'), header))
  fs.writeFileSync(path.join(o.out, 'report.json'), JSON.stringify({seed: o.seed, header, summary, unexpected: unexpected.map((r) => ({id: r.c.id, fields: r.diff})), known: results.filter((r) => r.gap).map((r) => ({id: r.c.id, gap: r.gap.id, fields: r.diff}))}, null, 2))

  if (o.filter.length === 0 && o.row.length === 0) {
    for (const g of gaps) if (!usedGaps.has(g.id)) console.error(`equivalence: warning: the known gap "${g.id}" matched no mismatch. Remove it if lexi now matches alex.`)
  }
  for (const g of gaps.filter((x) => usedGaps.has(x.id))) {
    const n = results.filter((r) => r.gap === g).length
    console.log(`known gap ${g.id}: ${n} case${n === 1 ? '' : 's'}. Parity.md: ${g.parity}. ${g.reason}`)
  }
  for (const r of unexpected) {
    console.log(`\nMISMATCH ${r.c.id}  (${r.diff.join(', ')})  args: ${JSON.stringify(r.c.args)}`)
    for (const f of r.diff) {
      console.log(`  ${f}, alex:\n${clip(r.na[f] ?? r.na.error ?? '')}\n  ${f}, lexi:\n${clip(r.nl[f] ?? '')}`)
    }
  }
  if (unexpected.length > 0) {
    console.error(`\nequivalence: ${unexpected.length} mismatch${unexpected.length === 1 ? '' : 'es'} that known-gaps.json does not list. Output: ${o.out}/mismatches`)
    return 1
  }
  console.error('equivalence: ok')
  return 0
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().then(
    (code) => process.exit(code),
    (e) => {
      console.error(`equivalence: ${e.stack ?? e}`)
      process.exit(2)
    }
  )
}
