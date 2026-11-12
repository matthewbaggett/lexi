// Rebuilds internal/rules/profanity.json from retext-profanities 7.2.2, cuss
// 2.2.0 and pluralize 8.0.0. See docs/Upstream.md.
//
// Run it from a directory where `npm install alex@11.0.1` ran:
//
//   LEXI=<this repo> node <this repo>/scripts/upstream/import-profanity.mjs
//
// It repeats the `unpack` step of retext-profanities/factory.js and the
// English config of retext-profanities/en.js, so the list holds each word of
// `cuss` together with its singular and plural form. JavaScript key order is
// kept.
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
import {pathToFileURL} from 'node:url'

// Packages resolve from the current directory, not from this script.
const nm = path.resolve('node_modules')
const {cuss} = await import(pathToFileURL(path.join(nm, 'cuss', 'index.js')))
const pluralize = createRequire(path.join(nm, 'x.js'))('pluralize')
const lexi = process.env.LEXI
if (!lexi) throw new Error('set LEXI')

// Same list as en.js.
const ignorePluralize = ['children', 'dy', 'pro', 'remain', 'so', 'dice', 'fus']
const result = {}
const add = (key, value) => {
  if (!ignorePluralize.includes(key)) result[key] = value
}
for (const key of Object.keys(cuss)) {
  add(key, cuss[key])
  add(pluralize.singular(key), cuss[key])
  add(pluralize.plural(key), cuss[key])
}
const lines = Object.entries(result).map(([k, v]) => '  ' + JSON.stringify([k, v]))
fs.writeFileSync(
  path.join(lexi, 'internal/rules/profanity.json'),
  '[\n' + lines.join(',\n') + '\n]\n'
)
console.log(lines.length + ' words')
