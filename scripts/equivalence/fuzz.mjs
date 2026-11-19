// The fuzz generator of the equivalence suite: random files, random configuration, random arguments.
// A seed fixes the whole run, so a mismatch is easy to repeat: `EQUIV_SEED=<seed> mise run equivalence`.
//
// It keeps away from the arguments that are known gaps and have cases of their own: --help,
// --version and an unknown reporter.

/** A small, fast generator with a 32-bit seed. */
export function mulberry32(seed) {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const pick = (rng, list) => list[Math.floor(rng() * list.length)]
const chance = (rng, p) => rng() < p
const some = (rng, list, max) => {
  const n = 1 + Math.floor(rng() * max)
  return Array.from({length: n}, () => pick(rng, list))
}

const PATHS = [
  'a.md', 'b.md', 'c.txt', 'd.text', 'e.markdown', 'f.html', 'g.mdx', 'h.htm', 'x.js', 'readme.ron',
  'docs/i.md', 'doc/j.txt', 'src/k.md', 'src/deep/l.md', 'src/m.mdx', '.hid/n.md', 'node_modules/o/p.md',
  'docs/q.html', 'UP.MD', 'sp ace.md'
]

const TEXT = [
  'He said she was fine.', 'His bike and her bag.', 'The hysterical crowd.', 'An asshat and a beaver.',
  'Man hours and mankind.', 'Fine text with no findings.', 'He is a addict and handicapped.',
  'The policeman and the fireman.', 'Turn it off and on, gals.', 'Do not be a retard.',
  'A white-list and a blacklist.', 'She is learning disabled.', 'Whore, hell and damn.', ''
]
const MD = [
  '# He heading', '> His quote', '- He item\n- Her item', '```\nHe code\n```', '`He`', '[He](https://x.test/he)',
  '| a | b |\n|---|---|\n| He | his |', '<!--alex ignore he-she-->', '<!--alex disable he-she-->',
  '<!--alex enable he-she-->', '<!--alex ignore-->', '<!--alex frob-->', '---\ntitle: He\n---', '**He** and _she_',
  '![He](x.png "his")', 'Line one  \nHe two', '1. He\n2. She'
]
const HTML = [
  '<p>He</p>', '<b>He<p>his</b>', '<!--alex ignore he-she--><p>He</p>', '<!--alex frob-->', '<ul><li>He<li>her</ul>',
  '<table><tr><td>He</table>', '<pre>He</pre>', '<script>He</script>', '<p>a &amp; He', '<!doctype html><title>He</title>'
]
const MDX = [
  'import X from "x"', 'export const a = 1', '<X he="1">His</X>', '{he}', '{/* alex ignore he-she */}',
  '<B>\n\nHe\n\n</B>', '{/* alex disable */}'
]

const CONFIGS = [
  ['.alexrc', () => '{}'],
  ['.alexrc', (r) => JSON.stringify({allow: some(r, ['he-she', 'invalid', 'gals-man', 'asshat', 'hell'], 2)})],
  ['.alexrc.json', (r) => JSON.stringify({deny: some(r, ['he-she', 'invalid', 'gals-man', 'hell'], 2)})],
  ['.alexrc.yml', (r) => `noBinary: ${chance(r, 0.5)}\n`],
  ['.alexrc.yaml', (r) => `profanitySureness: ${Math.floor(r() * 3)}\n`],
  ['.alexrc', (r) => JSON.stringify({profanitySureness: Math.floor(r() * 3), noBinary: chance(r, 0.5)})],
  ['package.json', (r) => JSON.stringify({name: 'x', alex: {allow: [pick(r, ['he-she', 'invalid'])]}})],
  ['package.json', () => '{"name":"x"}'],
  ['.alexrc', () => '{"allow":["he-she"],"deny":["invalid"]}']
]

const IGNORES = ['src', '*.txt', '!keep.txt', 'docs/', '*.md', 'a.md', '**/deep', '# comment\nb.md']

const FLAGS = [
  '-w', '-q', '-t', '-l', '-d', '--why', '--quiet', '--text', '--html', '--mdx', '--diff', '-wq', '-tq', '-lw',
  '--no-why', '--no-quiet', '--quiet=false', '--bogus', '--reporter=json', '--reporter', '-x'
]
const INPUTS = [
  '.', 'src', 'docs', 'doc', 'a.md', 'b.md', 'c.txt', 'f.html', 'g.mdx', 'missing.md', '*.md', '**/*.md',
  'src/**', '*.{md,txt}', '{a,b}.md', 'src/*.md', 'docs/**/*.html', '*.html', '!a.md', 'UP.MD', 'sp ace.md',
  'x.js', 'node_modules', '.hid', '.hid/n.md'
]

function body(rng, path) {
  const ext = path.slice(path.lastIndexOf('.'))
  const pool = ['.html', '.htm'].includes(ext) ? [...HTML, ...TEXT] : ext === '.mdx' ? [...MDX, ...MD, ...TEXT] : ext === '.txt' || ext === '.text' ? TEXT : [...MD, ...TEXT]
  const parts = some(rng, pool, 5)
  return parts.join(chance(rng, 0.2) ? '\r\n\r\n' : '\n\n') + (chance(rng, 0.8) ? '\n' : '')
}

/** Make `count` cases from a generator. */
export function fuzzCases(rng, count, seed) {
  const cases = []
  for (let n = 0; n < count; n++) {
    const files = {}
    for (const p of new Set(some(rng, PATHS, 6))) files[p] = body(rng, p)
    if (chance(rng, 0.2)) files['keep.txt'] = body(rng, 'keep.txt')
    if (chance(rng, 0.6)) {
      const [name, make] = pick(rng, CONFIGS)
      files[name] = make(rng)
    }
    if (chance(rng, 0.25)) files['src/.alexrc'] = pick(rng, ['{}', '{"allow":["he-she"]}', '{"noBinary":true}'])
    if (chance(rng, 0.3)) files['.alexignore'] = some(rng, IGNORES, 3).join('\n') + '\n'
    const args = [...new Set(chance(rng, 0.8) ? some(rng, FLAGS, 3) : [])]
    const stdin = chance(rng, 0.12)
    if (stdin) args.push('--stdin')
    else if (chance(rng, 0.85)) args.push(...new Set(some(rng, INPUTS, 3)))
    if (chance(rng, 0.2)) args.sort(() => rng() - 0.5)
    const env = {}
    if (chance(rng, 0.15)) env.FORCE_COLOR = pick(rng, ['1', '0', '2', 'true'])
    if (chance(rng, 0.1)) env.NO_COLOR = '1'
    cases.push({
      id: `fuzz/${seed}-${String(n).padStart(4, '0')}`,
      row: 'fuzz',
      files,
      args,
      env,
      stdin: stdin ? body(rng, 'stdin.md') : null,
      setup: null,
      tty: false
    })
  }
  return cases
}
