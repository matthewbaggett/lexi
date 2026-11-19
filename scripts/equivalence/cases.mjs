// Where the cases of the equivalence suite come from.
//
// 1. Case directories: testdata/equivalence/cases/<row>/<case>/. See testdata/equivalence/README.md.
// 2. Built-in generators: the format fixtures, one case for each rule, the profanity words, and the fuzz
//    generator. They need no files of their own.
//
// A case is { id, row, files, args, env, stdin, setup, tty }:
//   files  { "relative/path": string | Buffer }  the working directory before the run
//   args   string[]                              the arguments after the program name
//   env    { NAME: "value" }                     added to the clean environment
//   stdin  string | Buffer | null                what the program reads on stdin
//   setup  string | null                         a shell script, run in the directory before the program
//   tty    boolean                               give the program a terminal on stderr

import fs from 'node:fs'
import path from 'node:path'
import {fuzzCases} from './fuzz.mjs'

/** Read every file below a directory, dot files too. Keys are relative paths with `/`. */
export function readTree(dir) {
  const out = {}
  const walk = (rel) => {
    for (const entry of fs.readdirSync(path.join(dir, rel), {withFileTypes: true})) {
      const next = rel ? `${rel}/${entry.name}` : entry.name
      if (entry.isDirectory()) walk(next)
      else out[next] = fs.readFileSync(path.join(dir, next))
    }
  }
  if (fs.existsSync(dir)) walk('')
  return out
}

function readOptional(file) {
  return fs.existsSync(file) ? fs.readFileSync(file) : null
}

/** One argument per line. A trailing newline does not add an empty argument. */
export function parseArgs(text) {
  if (text === null) return []
  const lines = text.toString('utf8').split('\n')
  if (lines.at(-1) === '') lines.pop()
  return lines
}

export function parseEnv(text) {
  const env = {}
  if (text === null) return env
  for (const line of text.toString('utf8').split('\n')) {
    if (line === '' || line.startsWith('#')) continue
    const at = line.indexOf('=')
    if (at < 1) throw new Error(`bad env line: ${line}`)
    env[line.slice(0, at)] = line.slice(at + 1)
  }
  return env
}

/** Load the case directories of a root. */
export function loadDirCases(root) {
  const cases = []
  if (!fs.existsSync(root)) return cases
  for (const row of fs.readdirSync(root).sort()) {
    const rowDir = path.join(root, row)
    if (!fs.statSync(rowDir).isDirectory()) continue
    for (const name of fs.readdirSync(rowDir).sort()) {
      const dir = path.join(rowDir, name)
      if (!fs.statSync(dir).isDirectory()) continue
      const meta = readOptional(path.join(dir, 'case.json'))
      const opts = meta ? JSON.parse(meta.toString('utf8')) : {}
      const setup = readOptional(path.join(dir, 'setup.sh'))
      cases.push({
        id: `${row}/${name}`,
        row,
        files: readTree(path.join(dir, 'files')),
        args: parseArgs(readOptional(path.join(dir, 'args'))),
        env: parseEnv(readOptional(path.join(dir, 'env'))),
        stdin: readOptional(path.join(dir, 'stdin')),
        setup: setup ? setup.toString('utf8') : null,
        tty: Boolean(opts.tty)
      })
    }
  }
  return cases
}

function jsonl(dir) {
  const rows = []
  if (!fs.existsSync(dir)) return rows
  for (const file of fs.readdirSync(dir).filter((f) => f.endsWith('.jsonl')).sort()) {
    for (const line of fs.readFileSync(path.join(dir, file), 'utf8').split('\n')) {
      if (line) rows.push(JSON.parse(line))
    }
  }
  return rows
}

function config(row) {
  const cfg = {}
  for (const key of ['allow', 'deny', 'noBinary']) if (row[key] !== undefined) cfg[key] = row[key]
  if (row.sureness !== undefined) cfg.profanitySureness = row.sureness
  return Object.keys(cfg).length ? {'.alexrc': JSON.stringify(cfg)} : {}
}

/** Each fixture of testdata/formats, as a file argument and on stdin, with the flag of its kind. */
export function formatCases(repo) {
  const dir = path.join(repo, 'testdata', 'formats')
  const flag = {'.md': [], '.txt': ['--text'], '.html': ['--html'], '.mdx': ['--mdx']}
  const cases = []
  for (const name of fs.readdirSync(dir).sort()) {
    const kind = flag[path.extname(name)]
    if (!kind) continue
    const body = fs.readFileSync(path.join(dir, name))
    const base = {row: 'formats', env: {}, setup: null, tty: false}
    cases.push({...base, id: `formats/file-${name}`, files: {[name]: body}, args: [...kind, name], stdin: null})
    cases.push({...base, id: `formats/stdin-${name}`, files: {}, args: [...kind, '--stdin'], stdin: body})
  }
  return cases
}

/** One case for each of the equality rules, from the first positive case of the corpus. */
export function ruleCases(repo) {
  const seen = new Set()
  const cases = []
  for (const row of jsonl(path.join(repo, 'testdata', 'corpus', 'equality'))) {
    if (row.label !== 'positive' || !row.rule || seen.has(row.rule)) continue
    seen.add(row.rule)
    cases.push({
      id: `rules/${row.rule}`,
      row: 'rules',
      files: {'a.md': row.text, ...config(row)},
      args: ['a.md'],
      env: {},
      stdin: null,
      setup: null,
      tty: false
    })
  }
  return cases
}

/** The profanity cases of the corpus, each with its sureness, allow and deny options. */
export function profanityCases(repo, rng, count) {
  const rows = jsonl(path.join(repo, 'testdata', 'corpus', 'profanity'))
  const cases = []
  for (const sureness of [0, 1, 2]) {
    const pool = rows.filter((r) => r.sureness === sureness)
    for (let i = 0; i < count && pool.length > 0; i++) {
      const row = pool.splice(Math.floor(rng() * pool.length), 1)[0]
      cases.push({
        id: `profanity/sureness-${sureness}-${row.id}`,
        row: 'profanity',
        files: {'a.md': row.text, ...config(row)},
        args: ['a.md'],
        env: {},
        stdin: null,
        setup: null,
        tty: false
      })
    }
  }
  return cases
}

/** The equality cases of the corpus with options, sampled. */
export function optionCases(repo, rng, count) {
  const pool = jsonl(path.join(repo, 'testdata', 'corpus', 'equality')).filter(
    (r) => r.allow || r.deny || r.noBinary
  )
  const cases = []
  for (let i = 0; i < count && pool.length > 0; i++) {
    const row = pool.splice(Math.floor(rng() * pool.length), 1)[0]
    cases.push({
      id: `options/corpus-${row.id}`,
      row: 'options',
      files: {'a.md': row.text, ...config(row)},
      args: ['a.md'],
      env: {},
      stdin: null,
      setup: null,
      tty: false
    })
  }
  return cases
}

/** Every case of a run. */
export function allCases(repo, settings) {
  const {mulberry32} = settings
  return [
    ...loadDirCases(path.join(repo, 'testdata', 'equivalence', 'cases')),
    ...formatCases(repo),
    ...ruleCases(repo),
    ...profanityCases(repo, mulberry32(settings.seed), settings.corpus),
    ...optionCases(repo, mulberry32(settings.seed + 1), settings.corpus),
    ...fuzzCases(mulberry32(settings.seed + 2), settings.fuzz, settings.seed)
  ]
}
