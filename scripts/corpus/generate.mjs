// Builds the language-check corpus in testdata/corpus/ and records what the
// real alex 11.0.1 says about each text. The corpus is the golden file: tests
// run lexi over it and need no Node.
//
// Run it from a directory where `npm install alex@11.0.1` ran:
//
//   ALEX_ORACLE=<dir> node scripts/corpus/generate.mjs
//
// Each line of a testdata/corpus/<category>/<kind>.jsonl file is one case:
//   id        stable hash of the text and the options
//   category  "equality" for now. Other checks add their own value.
//   kind      the shape of the case, the same as the file name
//   label     "positive" (the target phrase should match), "negative" (it
//             should not), or "edge"
//   rule      the rule id the case aims at, when it has one
//   text      the input
//   allow, deny, noBinary   the alex options
//   want      what alex reported, one string for each finding
//
// The generator is deterministic: the same rules and the same alex give the
// same files.
import crypto from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
import {pathToFileURL} from 'node:url'

const oracle = process.env.ALEX_ORACLE
if (!oracle) throw new Error('set ALEX_ORACLE to a folder where alex@11.0.1 is installed')
const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..')
const {text: alexText} = await import(
  pathToFileURL(createRequire(path.join(oracle, 'x.js')).resolve('alex')).href
)
const rules = JSON.parse(fs.readFileSync(path.join(root, 'internal/rules/data.json'), 'utf8'))

// Seeded generator, so that runs repeat.
let seed = 0x1e51
const rnd = () => {
  seed = (seed + 0x6d2b79f5) | 0
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296
}
const pick = (list) => list[Math.floor(rnd() * list.length)]

const cases = []
const add = (kind, label, rule, text, options = {}) =>
  cases.push({kind, label, rule, text, ...options})

// A phrase with `*` wildcards becomes a real phrase.
const fill = (p) => p.replace(/\*/g, () => pick(['big', 'very', 'so']))
const cap = (s) => s.charAt(0).toUpperCase() + s.slice(1)
const filler = [
  'The weather was fine and the team shipped the release on time.',
  'Nobody had read the long design document before the meeting.',
  'She walked to the station while it rained on the quiet street.',
]

for (const r of rules) {
  const others = rules.filter((x) => x.id !== r.id)
  r.inconsiderate.forEach(([raw], i) => {
    const p = fill(raw)
    const trimmed = p.replace(/[.,]+$/, '')
    const id = r.id
    add('plain', 'positive', id, `We saw the ${p} today.`)
    add('capital', 'positive', id, `${cap(p)} is here`)
    add('upper', 'positive', id, `THE ${p.toUpperCase()} IS LOUD`)
    const punct = [`Did you say ${p}?`, `He said ${p}, then left.`, `Wait; ${p}; wait.`, `Really... ${p}!`]
    add('punctuation', 'positive', id, punct[i % punct.length])
    add('context', 'positive', id, `${pick(filler)} Then ${p} came up. ${pick(filler)}\n\n${pick(filler)}`)
    add('linebreak', 'positive', id, p.includes(' ') ? p.replace(/ /g, i % 2 ? '\n' : '\n\n') : `One line.\n${p}\nThree.`)
    add('hyphen', 'positive', id, p.includes(' ') ? `The ${p.replace(/ /g, '-')} part` : `A ${p}-based plan`)
    const inflect = [`${trimmed}s`, `${trimmed}es`, `${trimmed}'s`, `${trimmed}’s`, `${trimmed}ed`, `${trimmed}ing`]
    add('inflection', 'positive', id, `The ${inflect[i % inflect.length]} were here.`)
    add('quoted', 'negative', id, `The "${p}" key and the (${p}) label`)
    const look = [`${p}zzq`, `xx${p}`, `${p.split(' ')[0]}zzq ${p.split(' ').slice(1).join(' ')}`, `un${p}`]
    add('lookalike', 'negative', id, `We saw ${look[i % look.length]} today.`)
    const mix = ['\u{1F600}\u{1F600} ', 'café ☕ — ', '“It’s fine,” they said. ', '日本語 ']
    add('unicode', 'positive', id, `${mix[i % mix.length]}the ${p} here`)
    add('allowed', 'negative', id, `We saw the ${p} today.`, {allow: [id]})
    const other = pick(others).id
    add('deny', 'positive', id, `We saw the ${p} today.`, {deny: [other, id]})
    if (r.type === 'or') add('nobinary', 'positive', id, `The ${p} and the ${p} again`, {noBinary: true})
  })
}

// Pairs of phrases of one `or` rule and of different categories.
for (const r of rules.filter((x) => x.type === 'or')) {
  const byCat = {}
  for (const [p, c] of r.inconsiderate) (byCat[c] ||= []).push(fill(p))
  const cats = Object.keys(byCat)
  if (cats.length < 2) continue
  const [a, b] = [byCat[cats[0]][0], byCat[cats[1]][0]]
  for (const [joiner, k] of [[' and ', 'and'], [' or ', 'or'], ['/', 'slash'], [' ', 'space'], [', ', 'comma']]) {
    add('binary', 'positive', r.id, `${cap(a)}${joiner}${b} went home.`)
    add('binary', 'positive', r.id, `${cap(a)}${joiner}${b} went home.`, {noBinary: true})
  }
}

// Hand-written edge cases.
const edge = [
  '', ' ', '\n', '\n\n\n', '\t', '.', '...', '!?', ' ', '﻿', 'x',
  'The master key.', 'master', 'MASTER', 'Master', 'mAsTeR',
  'The master\r\nkey and the slave\r\nnode.', 'line master slave',
  'He said “master” to her.', '‘master’', '«master»', '„master“', '「master」',
  'master: the key', 'the key: master', 'a — master — b', 'master-slave', 'master–slave', 'master—slave',
  'master/slave', 'master / slave', 'master and slave', 'master or slave', 'master, slave', 'slave; master',
  '(master)', '[master]', '{master}', 'x (master) y', 'x, master, y',
  'him', 'her', 'him and her', 'he or she', 'he/she', 's/he', 'he-she',
  "ain't", "Ain’t", "it's the dummy's", 'dummy’s',
  'a.d.h.d.', 'A.D.H.D', 'ADHD', 'o.c.d.', 'O.C.D.',
  'Mr. Master', 'Dr. Slave and Mrs. Master', 'St. Master', 'e.g. master', 'i.e. dummy',
  'Hello.Master', 'Hello. master', 'Hello.\nmaster', 'one.two master', '1. master', '3.5 master',
  'mäster dummy', '\u{1F600} dummy', '\u{1F600}\u{1F600}\u{1F600} master slave', 'dummy \u{1F600} dummy',
  'café dummy', 'naïve dummy', 'dummy dummy dummy', 'dummy, dummy; dummy.',
  'D​ummy', 'dum­my', 'dummy​', 'ｍａｓｔｅｒ',
  'man' + ' man'.repeat(40), 'a '.repeat(2000) + 'dummy', 'dummy ' + 'b '.repeat(2000),
  ('The quick brown fox jumps over the lazy dog. ').repeat(300) + 'The master key.',
  ('Line of text without any hits.\n').repeat(500) + 'Last line has a dummy.',
  'x'.repeat(100000) + ' dummy', 'dummy' + ' '.repeat(5000) + 'master',
  'master ' + 'master '.repeat(200), '-'.repeat(500) + ' master', '"'.repeat(51) + 'master',
  '<b>master</b>', '`master`', '**master**', '# master', '> master', '- master', 'master\\slave',
  'http://example.com/master/slave', 'user@master.example', 'master_slave', 'masterSlave', 'MASTER_SLAVE',
  '1master', 'master1', 'master.', '.master', 'master...', '...master', 'master?', '?master', 'master!', '!master',
  'Mon Dieu, le maître', 'He is a man. She is a woman. They are people.',
  'Fireman, policeman, chairman and mailman.', 'a fire fighter', 'Girls and boys',
  'crazy\ninsane\nlame\nblind\ndeaf\ndumb', 'blindspot', 'blind spot', 'blind-spot', 'blind\tspot',
]
for (const t of edge) add('edge', 'edge', undefined, t)
for (const t of ['master slave', 'him her', 'he she', 'dummy variable']) {
  add('edge', 'edge', undefined, t, {noBinary: true})
  add('edge', 'edge', undefined, t, {allow: ['dummy']})
  add('edge', 'edge', undefined, t, {deny: ['master-slave']})
}

// Random mixes of phrases and separators, to stress the tokenizer.
const seps = [' ', '  ', '\n', '\r\n', '\n\n', '\t', '\u00a0', ', ', '. ', '.\n', '! ', '? ', '?! ', '... ', '\u2026 ', ': ', '; ',
  ' \u2014 ', ' \u2013 ', '-', '--', '/', ' / ', '\\', '&', ' & ', '_', '*', '+', '=', '#', '@', '%', '|', '~', '^', '`',
  '\'', '\u2019', '\u2018', '"', '\u201c', '\u201d', '\u00ab', '\u00bb', '(', ')', '[', ']', '{', '}', '<', '>', ' (', ') ', ' "', '" ',
  ' 3.5 ', ' 42 ', '1,000 ', ' \u{1F600} ', '\u00e9', ' \u65e5\u672c ', ' and ', ' or ', ' w/ ', ' vs. ', ' Mr. ', ' e.g. ', ' etc. ']
const pool = []
for (const r of rules) for (const [p] of r.inconsiderate) pool.push(fill(p))
pool.push('He', 'She', 'Mr', 'Dr', 'St', 'e.g', 'i.e', 'A.D.H.D', 'x', 'O', 'em', 'tis', 'the', 'a', 'is', 'and', 'or', 'not', 'it\u2019s', "don't")
for (let n = 0; n < 3000; n++) {
  let t = ''
  for (let k = 1 + Math.floor(rnd() * 8); k > 0; k--) {
    const w = pick(pool)
    t += (rnd() < 0.15 ? cap(w) : rnd() < 0.05 ? w.toUpperCase() : w) + pick(seps)
  }
  add('mixed', 'edge', undefined, t, rnd() < 0.2 ? {noBinary: true} : {})
}

// Record what alex says, then write one file for each kind.
const run = (c) =>
  alexText(c.text, {
    noBinary: c.noBinary,
    ...(c.allow ? {allow: c.allow} : {}),
    ...(c.deny ? {deny: c.deny} : {}),
  })
    .messages.filter((m) => m.source === 'retext-equality')
    .map(
      (m) =>
        `${m.line}:${m.column}-${m.position.end.line}:${m.position.end.column}: ${m.reason} [${m.ruleId}]`,
    )

const outDir = path.join(root, 'testdata', 'corpus', 'equality')
fs.rmSync(outDir, {recursive: true, force: true})
fs.mkdirSync(outDir, {recursive: true})
const files = {}
const seen = new Set()
let surprises = 0
for (const c of cases) {
  const out = {
    id: crypto.createHash('sha1').update(JSON.stringify([c.text, c.allow, c.deny, !!c.noBinary])).digest('hex').slice(0, 10),
    category: 'equality',
    kind: c.kind,
    label: c.label,
    ...(c.rule ? {rule: c.rule} : {}),
    text: c.text,
    ...(c.allow ? {allow: c.allow} : {}),
    ...(c.deny ? {deny: c.deny} : {}),
    ...(c.noBinary ? {noBinary: true} : {}),
  }
  if (seen.has(out.id)) continue
  seen.add(out.id)
  out.want = run(c)
  // Report cases whose label disagrees with alex. They stay in the corpus.
  const hit = out.rule && out.want.some((w) => w.endsWith(`[${out.rule}]`))
  if ((c.label === 'positive' && !hit && out.rule) || (c.label === 'negative' && hit)) surprises++
  ;(files[c.kind] ||= []).push(JSON.stringify(out))
}
for (const [kind, lines] of Object.entries(files)) {
  fs.writeFileSync(path.join(outDir, `${kind}.jsonl`), lines.join('\n') + '\n')
}
console.log(`${seen.size} cases in ${Object.keys(files).length} files; ${surprises} cases where the label and alex disagree`)
