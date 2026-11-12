// Rebuilds internal/rules/data.json and testdata/upstream.json from
// retext-equality 6.6.0. See docs/Upstream.md.
//
// Run it from a directory where `npm install alex@11.0.1` ran, so that
// `unified`, `retext-english` and `retext-equality` resolve:
//
//   UPSTREAM_PKG=<extracted npm package dir> UPSTREAM_TESTS=<test.js of the 6.6.0 tag> \
//   LEXI=<this repo> node <this repo>/scripts/upstream/import.mjs
import fs from 'node:fs'
import path from 'node:path'
import {pathToFileURL} from 'node:url'

const pkg = process.env.UPSTREAM_PKG
const tests = process.env.UPSTREAM_TESTS
const lexi = process.env.LEXI
if (!pkg || !tests || !lexi) throw new Error('set UPSTREAM_PKG, UPSTREAM_TESTS and LEXI')

// 1. The rules. Keep the order of the phrases: it decides the order of the
// findings.
const {patterns} = await import(pathToFileURL(path.join(pkg, 'lib', 'en.js')))
const rules = patterns.map((p) => {
  const r = {id: p.id, type: p.type}
  if (p.apostrophe) r.apostrophe = true
  r.inconsiderate = Object.entries(p.inconsiderate)
  if (p.considerate) r.considerate = Object.keys(p.considerate)
  if (p.condition) r.condition = p.condition
  if (p.note) r.note = p.note
  return r
})
fs.writeFileSync(
  path.join(lexi, 'internal/rules/data.json'),
  '[\n' + rules.map((r) => '  ' + JSON.stringify(r)).join(',\n') + '\n]\n'
)

// 2. The tests. Run upstream's test.js with a recording shim: each call of its
// `process` helper becomes one fixture.
const shim = fs
  .readFileSync(tests, 'utf8')
  .replace("import test from 'tape'", "import test from './.tape-shim.mjs'")
  .replace(
    "import {retext} from 'retext'",
    "import {unified} from 'unified'; import retextEnglish from 'retext-english'; " +
      'const retext = () => unified().use(retextEnglish).use(function () { this.Compiler = () => "" })'
  )
  .replace("from './index.js'", "from 'retext-equality'")
  .replace(
    'function process(doc, options) {',
    'function process(doc, options) {\n  globalThis.__last = {input: doc, options: options || {}}'
  )
fs.writeFileSync('.tape-shim.mjs', 'export const all = []\nexport default (name, fn) => all.push([name, fn])\n')
fs.writeFileSync('.upstream-test.mjs', shim)
const {all} = await import(pathToFileURL(path.resolve('.tape-shim.mjs')))
await import(pathToFileURL(path.resolve('.upstream-test.mjs')))
const cases = []
for (const [, fn] of all) {
  fn({
    same(actual, expected, name) {
      const last = globalThis.__last
      globalThis.__last = null
      if (last && Array.isArray(actual) && actual.every((m) => typeof m === 'string')) {
        cases.push({name, input: last.input, options: last.options, messages: actual})
      }
    },
    end() {}
  })
}
fs.writeFileSync(
  path.join(lexi, 'testdata/upstream.json'),
  '[\n' + cases.map((c) => '  ' + JSON.stringify(c)).join(',\n') + '\n]\n'
)
fs.rmSync('.tape-shim.mjs')
fs.rmSync('.upstream-test.mjs')
console.log(`${rules.length} rules, ${cases.length} fixtures`)
