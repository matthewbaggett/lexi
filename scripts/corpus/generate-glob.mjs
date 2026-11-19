// Builds the glob corpus in testdata/corpus/glob/ and records what the real alex 11.0.1
// expands for each pattern. The Go test TestGlobCorpus in internal/files replays the cases
// and needs no Node.
//
//   ALEX_ORACLE=$PWD/testdata/oracle node scripts/corpus/generate-glob.mjs
//
// Run it on Linux, where the file system is case sensitive. On macOS use Docker:
//
//   docker run --rm -v "$PWD":/w -w /w -e ALEX_ORACLE=/w/testdata/oracle node:25-slim \
//     node scripts/corpus/generate-glob.mjs
//
// alex 11.0.1 uses unified-engine 10.1.0, which calls glob 8.1.0 (minimatch 5.1.9) for each
// argument that glob.hasMagic accepts, and then searches the matches. The files are:
//
//   trees.json           the file trees: name -> list of entries. "a/b.md" is a file, "d/" a
//                        directory, {"link": "x", "to": "y"} a symbolic link.
//   <kind>.jsonl         one case per line:
//     id       hash of the tree and the pattern. Unique.
//     tree     the name of the tree. The test runs the pattern in <tree>/root.
//     kind     the shape of the pattern. It matches the file name.
//     pattern  the argument. ${ROOT} stands for the absolute path of the tree's root.
//     magic    what glob.hasMagic says. When false, alex does not call glob.
//     want     what glob returns, in order. Empty when nothing matches.
//     files    what the finder gives for the argument, as a sorted set, with the paths as alex prints them: "path" or "path !message".
//     crash    alex throws in the finder (the ignore package rejects a path): no files.
//     nofiles  true when a match is a directory with a link to a parent: the finder would not end.
//
// The generator is deterministic: the same code and the same alex give the same files.
import crypto from 'node:crypto'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import {createRequire} from 'node:module'
import {pathToFileURL} from 'node:url'

const oracle = process.env.ALEX_ORACLE
if (!oracle) throw new Error('set ALEX_ORACLE to a folder where alex@11.0.1 is installed')
const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..')
const req = createRequire(path.join(oracle, 'x.js'))
const glob = req('glob')
const minimatch = req('minimatch')
const {finder} = await import(pathToFileURL(req.resolve('unified-engine/lib/finder.js')).href)
const out = path.join(root, 'testdata', 'corpus', 'glob')

// ---------------------------------------------------------------- trees
const trees = {
  flat: [
    'a.md', 'b.md', 'c.md', 'ab.md', 'abc.md', 'a1.md', 'a2.md', 'a10.md', 'x.txt', 'y.js', 'z.json',
    '.hidden.md', '.a', '.b', 'a b.md', 'é.md', '日本.md', 'a.b.c.md', 'a-b.md', 'a_b.md', 'a+b.md',
    'a(b).md', 'a[b].md', 'a{b}.md', 'a!b.md', 'a@b.md', 'a,b.md', 'a$b.md', 'a^b.md', '#a.md',
    '!a.md', '~a.md', "a'b.md", 'ab', 'a.', '..a', '-a', '1', '2', '10', 'foo.bar.baz', 'a\\b.md',
    'a*b.md', 'a?b.md', 'a|b.md', 'a&b.md', 'a;b.md', 'a%b.md', 'a=b.md', '😀.md',
  ],
  nested: [
    'src/a.md', 'src/b.md', 'src/lib/c.md', 'src/lib/d.js', 'src/lib/deep/e.md', 'src/lib/deep/f.txt',
    'src/.hid/g.md', 'docs/readme.md', 'docs/guide/one.md', 'docs/guide/two.md', 'doc/x.md',
    'node_modules/pkg/index.md', 'node_modules/pkg/lib/a.md', 'test/t.md', 'test/t.test.md',
    '.config/h.md', '.config/sub/i.md', 'empty/', 'a/b/c/d/e/f.md', 'a/b/c/d/e/g.txt', 'a/b/x.md',
    'a/b/c/y.md', 'README.md', 'CHANGELOG.md', 'package.json', 'src/lib/deep/deeper/h.md',
    'src/lib.md', 'src.md', 'lib/src/a.md', 'lib/src/lib/b.md',
  ],
  dots: [
    '.a/.b/.c.md', '.a/x.md', '.x.md', 'x/.y.md', 'x/.y/z.md', 'x/y.md', '..foo', '.git/config',
    '.git/hooks/pre.md', '.hidden/', 'visible/.inner/deep.md', 'visible/file.md', '.a.b/c.md',
    '.dir.md/x.md', 'a/.b', 'a/b', '.', '.-x', '...', '.md', '..md',
  ].filter((e) => e !== '.'),
  links: [
    'real/a.md', 'real/b.md', 'real/sub/c.md', {link: 'link', to: 'real'}, {link: 'a.lnk', to: 'real/a.md'},
    {link: 'dead', to: 'nowhere'}, {link: 'real/inner', to: '../real'}, {link: 'loop', to: '.'},
    'plain/x.md', {link: 'plain/up', to: '..'}, {link: 'l2', to: 'link'}, {link: 'real/sub/back', to: '../..'},
    {link: 'abs', to: '__ROOT__/real'},
  ],
  unicode: [
    'a', 'B', 'c', 'A', 'b', 'C', '_x', '-y', '1', '10', '2', 'a b', 'a-b', 'ab', 'a.b', 'Ab', 'aB', 'AB',
    'é', 'e', 'f', 'z', 'Z', 'ä', 'ß', 'ss', 'ǆ', 'ñ', 'n', 'o', 'ø', 'ö', '_', '__', '~', '!', '@', '#',
    '$', '%', '&', '+', '=', 'a_b', 'a+b', 'a,b', 'a;b', 'ａ', '一', '二', 'あ', 'ア', 'ｱ', '😀',
    'x.md', 'X.md', 'x.MD', 'é.md', 'e.md', 'E.md', 'a1', 'a01', 'a2', 'a10', 'A1', 'ab1', 'a.1', 'a-1',
  ],
  ranges: [
    ...Array.from({length: 12}, (_, i) => `file${i + 1}.md`),
    ...Array.from({length: 10}, (_, i) => `f${String(i + 1).padStart(2, '0')}.md`),
    ...'abcdefghijklmnopqrstuvwxyz'.split('').map((c) => `${c}.md`),
    '-1.md', '0.md', '1.md', '2.md', '3.md', '4.md', '5.md', '6.md', '7.md', '8.md', '9.md', '10.md', '11.md',
    '1..3.md', 'a..c.md', '{1..3}.md', '{a,b}.md', 'x/1.md', 'x/2.md', 'x/3.md', 'x/a.md', 'x/b.md', 'x/c.md',
    '00.md', '01.md', '02.md', '03.md', '04.md', '05.md', '06.md',
  ],
  ext: [
    'foo.js', 'foo.min.js', 'foo.test.js', 'foo.md', 'bar.js', 'bar.min.js', 'baz.js.map', 'a.js', 'ab.js',
    'abc.js', 'aa', 'aaa', 'aab', 'abab', 'ababab', 'foo.bar', 'foo.bar.js', 'foo-bar', '(a)', '!(a)',
    '+a', '@a', '*a', '?a', 'a+', 'a@', 'a!', 'a*', 'a?', 'x+y.md', 'x@y.md', 'x!y.md', 'x(y).md',
    'x(y|z).md', 'xy.md', 'xz.md', 'xyz.md', 'x.md', 'y.md', 'z.md', 'js', 'md', 'a.md', 'a.txt',
    'b.md', 'b.txt', 'c.md', 'a/a.md', 'a/b.md', 'a/c.txt', 'b/a.md', 'b/b.js', 'ab/ab.md', 'ab/c.md',
    '.a.md', '.b.js', 'a/.c.md', 'foo/bar/baz.js', 'foo/bar.js', 'foo/baz.min.js', 'foo/test/a.test.js',
    'src/a.js', 'src/b.test.js', 'src/c.spec.js', 'src/d.md', 'test.js', 'test.test.js', 'tests.js',
  ],
  deep: [
    'a/a/a/a/a.md', 'a/a/a/b.md', 'a/a/b.md', 'a/b.md', 'b.md', 'a/a/a/a/', 'a/b/a/b/a.md', 'b/a/b/a/b.md',
    'x/a.md', 'x/y/a.md', 'x/y/z/a.md', 'x/y/z/w/a.md', 'x/y/z/w/v/a.md', 'x/.y/a.md', 'x/y/.z/a.md',
    'x/y/z/.w/a.md', 'one/two/three/four/five/six.md', 'one/two.md', 'one/two/three.md',
    'p/q/r/s.txt', 'p/q/r.md', 'p/q.md', 'p.md', 'q/p/q/p/q.md',
  ],
  dirs: [
    'd1/a.md', 'd1/b.txt', 'd2/c.md', 'd2/sub/d.md', 'd3/', 'd4/.x.md', 'f.md', 'd1.md', 'd2.d/x.md',
    'dir with space/a.md', 'd.md/inner.md', 'd.md/other.txt', 'e.txt/', 'file', 'file.d/a.md', 'z/',
    'z/y/', 'z/y/x/', 'z/y/x/w.md', 'g/h/i/j.md', 'g/h.md', 'g.md',
  ],
  case: ['A.md', 'a.md', 'B.MD', 'b.md', 'Dir/x.md', 'dir/x.md', 'DIR/Y.md', 'Foo.JS', 'foo.js', 'foo.Js', 'x/Y.md', 'x/y.md'],
}
const caseSensitiveOnly = new Set(['case'])

// ---------------------------------------------------------------- patterns
let seed = 0x91e7
const rnd = () => {
  seed = (seed + 0x6d2b79f5) | 0
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296
}
const pick = (a) => a[Math.floor(rnd() * a.length)]

// Patterns that run on every tree. Each group is a kind.
const fixed = {
  star: [
    '*', '**', '***', '*.md', '*.*', '*.', '.*', '.*.md', '*.md.', 'a*', '*a', 'a*b', 'a*c', '*a*', '*.js', '*.txt',
    '*.{md,txt}', '*/*', '*/*/*', '*/*/*/*', '*/**', '**/*', '*/', '*//', '**/*/**', '*/*.md', '*/.*', '.*/*',
    '*.md*', '*md', '**.md', 'a**', 'a**b', '**a', '*?', '?*', '?*?', '*?*', '**?', 'a*?', '*a?', '*****',
    '*.*.*', '*.**', '**.*', '*-*', '*_*', '* *', '*+*', '*(*', '*)*', '*[*', '*]*', '*{*', '*}*', '*,*',
  ],
  qmark: [
    '?', '??', '???', '?.md', '??.md', '?b.md', 'a?', 'a??', '?b', 'a?c', '?/?', '?/*', '*/?', '?.?', '.?', '?.',
    '?*', '????', '?????', '??????', '?/?/?', '?.js', '?.txt', '??/*', '?-?', '? ?', '?(?', '?)?',
  ],
  class: [
    '[abc]', '[a-c]', '[!a]', '[^a]', '[]a]', '[a-]', '[-a]', '[z-a]', '[[:alpha:]]', '[[:digit:]]*', '[[:upper:]]',
    '[a', 'a]', '[]', '[!]', '[\\]]', '[\\-]', '[a\\-c]', '[*]', '[?]', '[abc]*', '[a-c].md', '[!a-c].md', '[^a-c]*',
    '*[abc]', '*[a-c].md', '[a-c][a-c]', '[ab]?', '?[ab]', '[[]', '[]]', '[[]]', '[\\[]', '[a[]', '[a\\]]',
    '[.]', '[.]md', '*[.]md', '[!.]*', '[^.]*', '[[:alpha:]]*.md', '[[:digit:]].md', '[[:space:]]*', '[[:punct:]]*',
    '[[:alnum:]]', '[[:lower:]]*', '[[:word:]]', '[a-z]', '[A-Z]', '[A-z]', '[0-9]', '[0-9]*', '[a-cx-z]', '[--a]',
    '[a-a]', '[!-a]', '[\\!a]', '[a-\\z]', '[\\a-c]', '[/]', 'a[/]b', '[a/]', '[\\\\]', '[\\\\a]', '[é]', '[a-é]',
    '[😀]', '[^😀]', '*[😀]*', '[:alpha:]', '[[:alpha:]', '[[.a.]]', '[[=a=]]', '[!!]', '[!^]', '[^!]', '[a-b-c]',
    '[a-b-]', '[$]', '[(]', '[)]', '[|]', '[+]', '[{]', '[}]', '[,]', '[#]', '[ ]', '[^]', '[^]a]', '[!]a]',
    '[b-a]*', '[\\]', '[\\*]', '[\\?]', 'x[a-c', 'x[!', 'x[]', '*[[:alpha:]]', '[ä-ö]', '[a-]*', '[-]',
  ],
  brace: [
    '{a,b}', '{a,b}.md', '{a,b,c}*', 'a{,b}', '{a}', '{}', '{a,}', '{,a}', '{,}', '{a,b}{c,d}', 'x{a,b{c,d}}',
    '{*,.*}', '{a/b,c/d}', '{/*,}', 'a{b', 'a}b', '\\{a,b}', '{a,b\\}', '${a,b}', '$\\{a}', '{a..}', '{..a}',
    '{a,b}/*', '*/{a,b}.md', '{src,docs}/**/*.md', '{src,docs}/*', '{*.md,*.txt}', '{*.md,*.js}', '**/{a,b}.md',
    '{a,{b,c}}', '{{a,b},c}', '{{a,b}}', '{{a}}', 'a{{b}}c', '{a{b,c}d,e}', '{a,b,c,d,e,f}.md', '{a..c}',
    '{{a,b}}.md', 'a{b}c', '{a},b}', '{a}b,c}', 'a{b,c}}', '{a,b}}', '{{a,b}', '{a,b}{', '}{a,b}', '{a,b}{c',
    '{ab,a}.md', '{a,ab}.md', '{a,a}.md', '{.a,.b}', '{.*,*}', '{,.}*', '{x/,}*.md', '{docs/**/,doc/**/,}*.md',
    '{docs/**/,doc/**/,}*.{md,txt}', '*.{md}', '*.{md,}', '*.{,md}', '*.{md,md}', '{*,**}', '{**,*}/*', '{a,b}*/',
    'a,b', 'a{,}b', 'a{,,}b', '{a,,b}', '{,,}', '{x}{y}', '{x,}{y,}', '{a/,b/}*', '{a,b/}c', '{ab,}*', '{,ab}*',
    '\\{*', '\\}*', '{\\,}', '{a\\,b}', '{a\\,b,c}', '{\\a,b}', '{a,\\b}', '{a,b\\}c}', '{a\\}b,c}', '{{,a},b}',
    '{[ab],c}', '{[a,b]}', '{a,[b}]', '{(a),b}', '{a|b,c}', '{@(a|b),c}', '{*(a),b}', '{!(a),b}', '{?,??}',
    '{a,b}\\{', '\\\\{a,b}', '{a,b}\\', '{😀,é}', '{é,e}.md', '{a b,c}.md',
  ],
  range: [
    '{1..5}', '{05..10}', '{a..e}', '{e..a}', '{1..10..3}', '{a..e..2}', '{-3..3}', '{1..3}{a,b}', 'file{1..5}.md',
    'file{1..12}.md', 'file{01..12}.md', 'f{01..10}.md', 'f{1..10}.md', 'f{001..003}.md', '{a..c}.md', '{a..z}.md',
    '{A..C}', '{a..C}', '{1..1}', '{1..2..0}', '{1..2..-1}', '{3..1}', '{3..1..2}', '{3..1..-2}', '{a..a}', '{-1..1}.md',
    '{-01..01}.md', '{0..9}.md', '{1..3}/*', 'x/{1..3}.md', 'x/{a..c}.md', '{1..3}.{md,txt}', '{1.5..3}', '{1..3.5}',
    '{1..a}', '{a..1}', '{1..}', '{..1}', '{1...3}', '{1..3..}', '{1..3..x}', '{x..y..z}', '{1..3,5}', '{1..3,a}',
    '{1,2..4}', '{a,1..3}', '{1..3}{1..3}', '{1..2}{a..b}{3..4}', '*{1..3}*', '{10..1..3}', '{1..100}', '{1..20..5}.md',
    '{a..e..0}', '{a..e..-1}', '{e..a..-2}', '{a..e..10}', '{ab..cd}', '{1..3}x{4..6}', '{x..z}', '{X..Z}', '{Z..a}',
    '{y..B}', '{_..a}', '{[..a}', '{\\..a}', '{`..b}', '{1..9}.md', '{9..1}.md', '{00..02}.md', '{0..02}.md',
    '{1..03}.md', '{01..3}.md', '{-5..-1}', '{-1..-5}', '{-1..5..2}', '{- 1..2}', '{1..2}}', '{{1..2}', '{{1..2}}',
    '{1..2,}', '{,1..2}', '{a..c}{1..3}', '{1..3}*', '{1..3}?', '{1..3}[0-9]',
  ],
  extglob: [
    '?(a|b)', '*(a|b)', '+(a|b)', '@(a|b)', '!(a|b)', '?(a)', '*(a)', '+(a)', '@(a)', '!(a)', '?()', '*()', '+()',
    '@()', '!()', '?(a|)', '*(a|)', '+(a|)', '@(a|)', '!(a|)', '@(|)', '+(|)', '!(|)', '+(*.js)', '!(*.js)', '*(*.js)',
    '+(foo|bar).js', '*.+(js|md)', '*.@(js|md)', '*.!(js|md)', '*.?(js|md)', '*.*(js|md)', '+(a|b)*', '*+(a|b)',
    'foo.@(min|test).js', 'foo.!(min|test).js', 'foo.+(min|test).js', 'foo.?(min|test).js', 'foo.*(min|test).js',
    '+(a|@(b|c))', '@(a|b)@(c|d)', '!(a)!(b)', '!(!(a))', '+(+(a))', '*(*(a))', '@(@(a))', '?(?(a))', '+(a|b', '*(a|b',
    '@(a', '!(a', '?(a', '+(', '*(', '@(', '!(', '+(a|b)(c', 'a)', 'a|b)', '(a|b)', '(a)', '|', 'a|b', '(|)', '()',
    '@(a|b|c)', '@(a|b|c|d)', '!(a|b|c)', '+(a|b|c)', '*(a|b|c)*', '*(a|b|c)', '+(a|ab)', '+(ab|a)', '@(a|ab)', '@(ab|a)',
    '+(ab)', '+(ab)*', '*(ab)', '*(ab).md', '@(ab)*', '!(ab)*', '!(ab).md', '!(a).md', '!(*).md', '!(*)', '!(?)', '!(??)',
    '@(.a|.b)', '.@(a|b)', '@(*.md|*.txt)', '!(*.md|*.txt)', '!(*.md)*', '*!(.md)', '*.!(md)', '*.!(md|txt)', '!(*.md).*',
    '!(.*)', '!(.a)', '+(.a)', '@(.*)', '*(.*)', '.!(a)', '.+(a)', '.*(a)', '.?(a)', '.@(a)', '!(a)/*', '*/!(a)', '!(a)/**',
    '**/!(a)', '**/!(*.md)', '**/+(a|b).md', '**/@(a|b)/*', '+(a|b)/*', '@(a|b)/**/*.md', '@(a/b|c)', '@(a/|b)', '+(a/)',
    '!(a/b)', '!(a|b/c)', '@(a|b)/', '+(a|b)/', '!(a)/', '!(a|b)/', '*(a|b)/', '?(a|b)/', '@(*)', '+(*)', '*(*)', '?(*)',
    '@(?)', '+(?)', '*(?)', '!(?)*', '@([ab])', '+([ab])', '!([ab])', '@([a-c]|x)', '+([[:alpha:]])', '+([a-c])*', '@(\\*)',
    '+(\\()', '@(\\))', '@(\\|)', '@(a\\|b)', '+(a\\)b)', '\\+(a)', '\\@(a|b)', '\\!(a)', '\\?(a)', '\\*(a)', '+\\(a)', '+(a\\)',
    'a+(b)', 'a@(b)c', 'a!(b)c', 'a?(b)c', 'a*(b)c', '*+(b)', '@+(a)', '!+(a)', '+!(a)', '++(a)', '**(a)', '*@(a)', '@*(a)',
    '?+(a)', '+?(a)', '?!(a)', '!?(a)', '+@(a)', '@@(a)', '@!(a)', '!@(a)', '!!(a)', '!!!(a)', '!(a)(b)', '+(a)(b)', '@(a)(b)',
    '+(a)+(b)', '@(a)?(b)', '?(a)@(b)', '*(a)*(b)', '*(a)+(b)', '!(a)*', '*!(a)', '*!(a)*', '!(a)!(a)', '!(x)y', 'x!(y)', 'x!(y)z',
    '(a)+(b)', '+(a)(b)+(c)', '+(a|b)|c', '+(a|b)|', '|+(a|b)', '+(a|b)(', '+(a|b))', '+((a|b))', '+(a|(b))', '+(a|(b|c))',
    '@((a)|b)', '+(a(b)|c)', '+(a|b)c)', '@(a))', '@(a)(', '!(a|b)(', '!((a|b))', '!(a|(b))', '!(a(b))', '!(a(b)c)',
    '+([)a])', '+([(a])', '+(a[|]b)', '@(a[)])', '@([[:alpha:]|x])', '+(a|[b|c])', '+([a|b])', '+([a|b]|c)',
    '*.@(js|md|txt)', '*.+(js|md|txt)', '*.!(js|md|txt)', '+(x|y)+(z)', '@(x|xy)@(z|yz)', '+(x|y|z)', '*(x|y|z)',
    '+(x)(y)', '+(x)\\(y)', '+(x(y))', 'x+(y|z)', 'x@(y|z)', 'x*(y|z)', 'x?(y|z)', 'x!(y|z)', 'x+(y|z).md', 'x!(y|z).md',
    'x(y).md', 'x\\(y\\).md', 'x(y|z).md', 'x\\(y|z\\).md', '+(?)(?)', '@(??|?)', '!(a)(b)', '+@(a)', '@+(a)',
  ],
  globstar: [
    '**', '**/', '/**', '**/**', '**/**/**', '**/*', '**/*.md', '**/.*', '**/.*/**', '**/.*/*', '.**', '.**/*',
    '**/.**', '**/*/', '**/**/', '**/*.md/', '**/a', '**/a/', '**/a/**', '**/a/*', '**/a/**/b', '**/a/**/b/**', 'a/**',
    'a/**/', 'a/**/**', 'a/**/*', 'a/**/*.md', 'a/**/b', 'a/**/b/**', 'a/**/b/**/*.md', 'a/**/**/b', 'a/**/**/**/b',
    '*/**', '*/**/', '*/**/*', '*/**/*.md', '*/**/*/', '**/*/*', '**/*/*/*', '**/*/**/*', '**/*/**/*.md', 'src/**',
    'src/**/', 'src/**/*', 'src/**/*.md', 'src/**/lib/**/*.md', 'src/**/lib', 'src/**/lib/', 'src/**/lib/**', 'src/**/**',
    '**/src', '**/src/', '**/src/**', '**/src/*', '**/lib/**/*.md', '**/deep/**', '**/deep', '**/node_modules/**',
    '**/node_modules', 'node_modules/**', 'node_modules/**/*.md', '.config/**', '.config/**/*.md', '.config/**/', '.*/**',
    '.*/**/*.md', '**/.config/**', '**/.config', '**/.hid/**', 'src/.hid/**', 'src/**/.hid/**', 'src/**/.hid', '**.md', 'a**',
    '**a', 'a**/', 'a**/*', '**a/*', '**/a**', '**/**a', 'a/**b', 'a/b**', 'a**/b', '**/**.md', '**/*.md**', '***/*.md',
    '***', '****', '**/***', '***/**', '*/***/*', '**/*/***', '**//**', '**///**', '**/./**', '**/../**', './**', './**/',
    './**/*.md', '**/./*', './**/./*.md', 'a/./**', '**/.', '**/..', '**/.?*', '**/..?*', '**/*/.', '**/@(a|b)', '**/+(a)',
    '**/{a,b}', '**/{a,b}/**', '{**,a}', '{**/a,b}', '**/[ab]', '**/[ab]/**', '**/?', '**/??', '**/?/**', '**/?.md', '**/a?',
    '**/*[ab]', '**/**/a', '**/a/**/a', '**/a/**/a/**', '**/a/**/a/**/a', '**/a/**/b/**/a', 'x/**/a.md', 'x/**/**/a.md',
    'x/**/y/**/a.md', 'x/**/y/**/z/**/a.md', 'x/**/z/**/a.md', 'x/**/y/**/w/**/a.md', 'x/y/**/a.md', 'x/y/z/**/a.md',
    'x/y/z/**', 'x/y/z/**/', 'x/**/z/', 'x/**/z', 'x/**/z/*', 'x/**/z/**', '*/y/**', '*/y/**/a.md', '*/*/**', '*/*/**/a.md',
    '**/y/**/a.md', '**/y/z/**', '**/z/w/**', '**/w/**', '**/w', '**/v/**', '**/*/a.md', '**/*/*/a.md', '**/*/*/*/a.md',
    '**/a.md', '**/b.md', '**/b/**/a.md', '**/*.txt', '**/*.{md,txt}', '**/*.@(md|txt)', '**/*.!(md)', '**/!(*.md)',
    '**/!(*.md)/**', '!(**)', '**/!(**)', '**/!(a)/**', 'a/!(**)', '@(**)', '@(**)/*', '+(**)', '*(**)/a', '**(a)', '**(a|b)',
  ],
  dot: [
    '.', '..', '...', './', '../', './/', './.', './..', '../..', './*', '../*', '../*/', '../root/*', '../root/**',
    '../root/**/*.md', '../../*', '.*', '.*/', '.*/*', '.*/**', '.*.md', '*/.*', '*/.*/*', '*/.*/**', '**/.*', '**/.*/*',
    '.a', '.a/*', '.a/.*', '.a/**', '.a/**/*', '.a/.b', '.a/.b/*', '.a/.b/.*', '.a/.b/**', '.hidden*', '.hid*', '.h*', '.?', '.??',
    '.[a-z]*', '.[!.]*', '.[^.]*', '..?*', '..*', '.\\*', '.\\.', '\\.*', '\\.a', '\\..', '\\./*', '.{a,b}', '.{a,b}/*', '{.a,.b}',
    '{.,}*', '{.*,*}', '.@(a|b)', '@(.a|.b)', '.!(a)', '!(.a)', '.+(a)', '.*(a)', '.?(a)', '*.', '*..', '*..*', '.git', '.git/*',
    '.git/**', '*/config', '.*/config', '.git/config', '.g*/c*', '[.]*', '[.]a', '[.]*/*', '\\.*/*', '?a', '?hidden*', '*hidden*',
    '*.md', '.md', '..md', '.*.*', '.*.*.*', './.*', './.*/*', './*/.*', './*/*/.*', '../root/.*', '../root/.*/*', '../.',
    '../..', '../../..', '..//', '../root', '../root/', '../ro*', '../r?ot/*', '../*/*', '../**/*.md', '../**', '..//root/*',
    './/*', './/*', '././*', './././*', '*/./*', '*/../*', '*/./*/..', './*/../*', './a/../*', 'a/../*', 'a/./*', 'a/*/../*',
    'a/*/..', 'a/..', 'a/../a', '*/..', '*/../', '**/..', '.*/..', '.a/..', '.a/../*', '.a/./*', '.a/.', '.a/./.b', '.a/..a',
  ],
  escape: [
    '\\*', '\\?', 'a\\*b', '\\\\', 'a\\', '\\a', '\\[a]', '[a\\]]', '\\(', '\\!a', '\\*.md', 'a\\*', 'a\\?', 'a\\[b\\].md',
    'a\\{b\\}.md', 'a\\(b\\).md', 'a\\!b.md', 'a\\@b.md', 'a\\,b.md', 'a\\$b.md', 'a\\^b.md', '\\#a.md', '\\!a.md', '\\~a.md',
    "a\\'b.md", 'a\\\\b.md', 'a\\\\*', 'a\\\\?', 'a\\\\\\*', 'a\\\\\\\\b.md', '\\\\\\\\', '\\\\\\', '*\\', '?\\', '[\\', '\\[',
    '\\[*', '[\\*', '\\{', '\\}', '\\{*', '\\}*', '\\|', '\\+', '\\@', '\\.', '\\-', '\\ ', '\\/', 'a\\/b', '\\/*', '*\\/',
    '*\\/*', 'a\\ b.md', 'a\\ *', '\\é', '\\😀', '\\*\\*', '\\*\\?', '\\?\\*', '*\\*', '*\\?', '?\\*', '?\\?', '*\\**', '\\**',
    '\\??', 'a\\|b.md', 'a\\&b.md', 'a\\;b.md', 'a\\%b.md', 'a\\=b.md', 'a\\+b.md', 'a\\_b.md', 'a\\-b.md', 'a\\.b.md',
    '\\a\\.\\m\\d', '\\*\\.md', 'a\\b', 'a\\\\b', 'a\\\\\\b', '\\a*', '\\a?', '\\a[b]', '\\a{b,c}', '\\a+(b)', '\\a!(b)',
    'src\\/*', 'src\\\\*', 'src/\\*', 'src/\\**', 'src/\\*/*', '\\s*', '\\s\\r\\c/*', '\\.config/*', '\\.c*/*', '\\n*', '\\t*',
    '[\\a-c]', '[a-\\c]', '[\\]a]', '[\\^a]', '[a\\-]', '[\\-a]', '[\\\\]', '[\\!a]', '[!\\a]', '\\!(a)', '\\@(a)', '\\+(a)',
  ],
  slash: [
    '*/', '*//', '//*', '/*//', 'a//b', '//', '///', '*/*/', '*/*//', 'a/', 'a//', 'a/b/', 'a//b/', 'a/*/', 'a/**/', 'a/**/*/',
    '*.md/', '*.md//', '*/.', '*/./', '*/..', '*/../', '**/', '**//', '**/*/', '**/*//', './', './/', './/', './/.', './/*', './/*/',
    '*/*/*/', 'src/', 'src//', 'src/*/', 'src//*', 'src/*//', 'src/**/', 'src//**', 'src/**//', 'src/*/*/', 'src/*/*//', '/src/*',
    'src/lib/', 'src/lib//', 'src//lib/', 'src/lib/*/', 'src/lib/**/', 'src/lib/deep/', '?/', '??/', '?//', '[a-z]/', '[a-z]//',
    '{a,b}/', '{a,b}//', '{a/,b/}', '{a//,b/}', '{a,b}/*/', '+(a)/', '@(a|b)/', '!(a)/', '*(a)/', '?(a)/', 'a*/', 'a*//', '*a/',
    '*a//', 'a?/', 'a?//', '*/*.md/', '*/*.md//', '**/*.md/', '**/*.md//', '*.md/*', '*.md/*/', '*.md/**', '*.md/**/', '*.md/**/*',
    '.*/', '.*//', '.*/*/', '*/.*/', '.a/', '.a//', '.a/.b/', '.a/*/', '.a/**/', '..//*', '../*/', '../*//', '.././*', '.././/*',
  ],
  magicedge: [
    '', ' ', '  ', '!', '!!', '!!!', '!a', '!!a', '!*', '!*.md', '!(a)', '#', '#a', '#*', '# *', '*#', '~', '~/*', '~*', '$*', '$HOME/*',
    '*$', '%*', '&*', '=*', '*=*', '@*', '@', '+', '+*', '+(', '-*', '--*', '*-', '-', ',', ',*', '*,', ';*', '*;', "'*", "*'", '"*',
    '*"', '`*', '*`', '<*', '>*', '|*', '*|', '^*', '*^', '(*', '*)', '()*', '(*)', '[*', '*]', '[*]', '{*', '*}', '{*}', 'é*', '*é',
    '日*', '*本*', '😀*', '*😀', '?😀', '??', '?????????', 'a b*', '* b*', '*a b*', '* *', '* * *', ' *', '* ', '\t*', '*\t', '\n*',
    '*\n', 'a\n*', '\r*', '\\', '\\\\', '\\\\\\', '\\\\\\\\', 'a**b/c', '*a*b*c*', 'a*b*c', '*a*b*', '**a**', '*a**b*', 'a*a*a*a',
    '*.*.*.*', '*.*.*.*.*', '*..*', '*.', '.', '*/.*/*', 'index.*', '*.*.*', '*/*/*/*/*/*/*', '*****.md', '?????.md', '*???*',
    '[', ']', '[]', '[[', ']]', '[[]]', '[[]', '[]]', '[]]]', '[a', '[a-', '[a-]', '[a-z', '[!', '[^', '[!a', '[^a', '[!]', '[^]', '[!]]',
    '[^]]', '(', ')', '()', '(((', ')))', '(|)', '(a|b)', '(a|b', 'a|b)', '[(]', '[)]', '[|]', '[(|)]', '{', '}', '{}', '}{', '{{', '}}', '{{}}',
    '{a', 'a}', '{,', ',}', '{,}', '{a,', ',a}', '*{', '}*', '*{}*', '{}*', '*{}', 'a{}b', 'a{,}b', '{a,b', 'a,b}', '${', '${}', '${a}', '$*',
    'a\\', 'a\\\\', 'a\\*\\', '*a\\', '[a\\', '[a\\]', '[a\\]]', '{a\\', '{a\\,', '{a\\,b', '{a\\,b}', '(a\\', '+(a\\', '@(a\\|', '@(a\\)',
    'a'.repeat(300) + '*', '*' + 'a'.repeat(300), '?'.repeat(40), '*'.repeat(100), '*/'.repeat(4), '*/'.repeat(4) + '*.md',
    '**/'.repeat(3), '**/'.repeat(3) + '*.md', '*/**/'.repeat(2) + '*.md', '{a,b}'.repeat(6), '{a,b}'.repeat(6) + '*', '+(a)'.repeat(5),
    '*(a)'.repeat(5), '!(a)'.repeat(4), '[a-c]'.repeat(5), '?(a)'.repeat(5), '@(a|b)'.repeat(5), '*.*.*.*.*.*.*.*.*.*.md',
    '*a*a*a*a*a*a*a*a*a*b', '+(a|aa)+(a|aa)+(a|aa)+(a|aa)b', '*(a|aa)*(a|aa)*(a|aa)*(a|aa)b', '*(*(*(a)))b',
  ],
  mixed: [
    'src/*.md', 'src/**/*.md', 'src/lib/*.md', 'src/*/*.md', 'src/{a,b}.md', 'src/[ab].md', 'src/?.md', 'src/@(a|b).md',
    'src/+(a|b).md', 'src/!(a).md', 'src/*(a|b).md', 'src/?(a).md', '{src,docs}/*.md', '{src,docs}/**/*.md', '{src,docs}/*/*.md',
    'docs/*.md', 'docs/**', 'docs/**/*', 'docs/guide/*', 'docs/*/*', 'doc*/*.md', 'doc?/*.md', 'doc{,s}/*.md', 'doc{,s}/**/*.md',
    'test/*.md', 'test/*.test.md', 'test/!(*.test).md', 'test/*.!(test).md', 'a/b/c/d/e/*.md', 'a/b/c/d/*/*.md', 'a/*/c/*/e/*.md',
    'a/**/e/*.md', 'a/**/f.md', 'a/**/*.md', 'a/*/**/*.md', 'a/b/**/*.md', 'a/b/c/**', '*/b/*/d/*/*.md', '*/*/*/*/*/*.md',
    'README*', 'CHANGELOG*', '*.json', '*.{json,md}', 'R*.md', '[RC]*.md', '+(README|CHANGELOG).md', '@(README|CHANGELOG)*',
    '!(README|CHANGELOG).md', '!(README|CHANGELOG)*', 'src/lib/deep/*', 'src/lib/deep/**', 'src/lib/**/*.txt', 'src/**/*.txt',
    '**/*.txt', '**/deep/*.md', '**/deep/**/*.md', '**/lib/*.md', '**/lib/**', 'lib/**/lib/*.md', 'lib/*/lib/*.md', 'lib/**/*.md',
    '*/lib/*.md', '*/lib/**', '*/src/*.md', '*/src/*/*.md', '*/*/a.md', '*/*/*.md', '*/*/*/*.md', '*/*/*/*/*.md', 'd?/*.md', 'd*/*',
    'd[12]/*', 'd[!1]/*', 'd{1,2}/*', 'd{1,2}/*.md', 'd@(1|2)/*', 'd+(1|2)/*', 'd!(1)/*', 'd*(1)/*', 'd?(1)/*', '*.d/*', '*.d/**',
    'd*/*/*', 'd*/**', 'd*/**/*.md', '*/*.md', '*/*.txt', '*/*', '*/**/*', 'file*', 'file?', 'file??', 'file*.md', 'file?.md',
    'file??.md', 'file[1-5].md', 'file[!1-5].md', 'file1*.md', 'file[1-9]*.md', 'f0*.md', 'f[0-9][0-9].md', 'x/*.md', 'x/?.md',
    'x/[a-c].md', 'x/{a,b}.md', 'x/@(1|2).md', 'x/**/*.md', 'real/*', 'real/**', 'real/**/*.md', 'link/*', 'link/**', 'link/**/*.md',
    'loop/*', 'loop/**', 'loop/*/*', 'loop/*/*/*', 'loop/**/*.md', '*/*.md', '**/*.md', '*/**', 'plain/*', 'plain/**', 'plain/up/*',
    'plain/up/**', 'plain/up/plain/*', 'real/inner/*', 'real/inner/**', 'real/**/inner', 'real/sub/*', 'real/sub/**', 'real/sub/back/*',
    'real/sub/back/**', 'real/*/*', 'real/*/**', 'a.lnk', 'a.l*', 'dead', 'dea?', 'dead*', '*ead', 'l2/*', 'l2/**', 'l?', 'l*', 'abs/*',
    'abs/**', '*/real/*', '**/real/**', '**/link/**', '**/up/**', '**/back/**', '**/inner/**', '**/loop/**', 'z/**', 'z/**/*.md',
    'z/*/*', 'z/y/*', 'z/y/**', 'z/y/x/*', 'z/y/x/**', 'z/*/x/*', 'z/**/w.md', 'g/**/*.md', 'g/*/*.md', 'g/*/*/*.md', 'g*', 'g*/*',
    'e.txt', 'e.t*', 'e.txt/', 'e.txt/*', 'file', 'file*', 'file/*', 'file.d/*', 'd.md/*', 'd.md/*.md', 'd*.md/*', 'd?.md', '*.md/*.md',
    'dir with space/*', 'dir*/*', 'dir?with?space/*', 'dir\\ with\\ space/*', '[d]ir*/*', 'a b*', 'a?b*', 'a\\ b*', 'a b.md', 'a?b.md',
    '**/a b*', '**/*b.md', '**/*.{md,txt}', '**/*.@(md|txt)', '**/!(*.md|*.txt)', 'x*/*', 'x*/**', 'x/!(a)*', 'x/+(a|b)*',
  ],
  dirmatch: [
    '*', '*/', 'd*', 'd*/', 'd?', 'd?/', 'd[12]', 'd[12]/', 'd[12]*', 'd{1,2}', 'd{1,2}/', 'z', 'z*', 'z/', 'z/*', 'z/*/', 'z/*/*', 'z/*/*/',
    'z/*/*/*', 'z/**', 'z/**/', 'z/**/*', 'z/**/*/', '*/*/', '*/*/*/', 'g/*', 'g/*/', 'g/*/*', 'g/*/*/', 'g/**', 'g/**/', 'g*/', 'g*/*/',
    'e.txt*', 'e.txt/', 'e.txt/*', 'e.*', 'e.*/', 'e.t?t', 'e.t?t/', 'd.md', 'd.md/', 'd.md*', 'd.md*/', 'd.md/*', 'd.md/*/', 'd?md',
    'd?md/', 'd.m?', 'd.m?/', '*.md', '*.md/', '*.md/*', '*.d', '*.d/', '*.d/*', 'file*', 'file*/', 'file.d*', 'file.d*/', 'file?d', 'file?d/',
    'empty*', 'empty*/', 'empty/*', 'empty/**', 'empty?', 'empty?/', 'src*', 'src*/', 'src?', 'src?/', 'src.*', 'src.*/', 'doc?', 'doc?/',
    'doc*', 'doc*/', 'doc{,s}', 'doc{,s}/', 'a*', 'a*/', 'a?', 'a?/', 'a/*', 'a/*/', 'a/b*', 'a/b*/', 'a/b/*', 'a/b/*/', 'a/b/c*', 'a/b/c*/',
    'a/*/*', 'a/*/*/', 'a/*/*/*', 'a/*/*/*/', 'a/*/*/*/*', 'a/*/*/*/*/', 'a/*/*/*/*/*', '**/a/', '**/a', '**/*/', '**/?/', '**/??/',
    'lib*', 'lib*/', 'lib?', 'lib?/', 'real*', 'real*/', 'link*', 'link*/', 'link', 'link/', 'link/*', 'link/*/', 'loop', 'loop/', 'loop/*',
    'loop/*/', 'l*', 'l*/', 'l?', 'l?/', 'plain*', 'plain*/', 'plain/*', 'plain/*/', 'plain/up*', 'plain/up*/', 'plain/up/*', 'plain/up/*/',
    'dead*', 'dead*/', 'dead?', 'dead?/', 'a.l*', 'a.l*/', 'x*', 'x*/', 'x/*', 'x/*/', 'x/*/*', 'x/*/*/', 'x/*/*/*', 'x/*/*/*/',
    '.*', '.*/', '.?', '.?/', '.*/*', '.*/*/', '.a*', '.a*/', '.a/*', '.a/*/', 'vis*', 'vis*/', 'visible/*', 'visible/*/', 'visible/.*',
    'visible/.*/', 'visible/.*/*', '**/.*/', '**/.*', '**/.inner', '**/.inner/', '**/.inner/*', '**/.inner/*/',
  ],
  caseless: [
    'a.md', 'A.md', '*.MD', '*.md', '[aA].md', '[Aa].md', 'dir/*', 'Dir/*', 'DIR/*', 'dir/*.md', 'Dir/*.md', '[dD]ir/*', '[dD]*/*', '*/*.md', '*/*.MD',
    '*/[xyXY].md', 'foo.*', 'Foo.*', 'foo.[jJ]s', 'foo.@(js|JS|Js)', 'foo.+(js|Js|JS)', '[fF]oo.*', 'f*', 'F*', '[a-z].md', '[A-Z].md', '[a-zA-Z].md',
    '[[:upper:]].md', '[[:lower:]].md', '[!a-z].md', '[^a-z].md', '[!A-Z].md', '**/*.md', '**/*.MD', '**/Y.md', '**/y.md', '**/[yY].md', '*.{md,MD}',
    '{a,A}.md', '{B,b}.{MD,md}', 'x/*', 'x/Y*', 'x/y*', 'x/[Yy]*', '?.md', '?.MD', '?.??', '???', '????.md',
  ],
}

// A random token for the fuzz patterns.
const tokens = [
  '*', '*', '*', '**', '?', '?', '.', '.', '/', '/', '/', '[a-c]', '[!a]', '[abc]', '[a', ']', '{a,b}', '{a,b,c}', '{1..3}', '{,}',
  '+(a|b)', '@(a|b)', '!(a)', '*(a|b)', '?(a)', '+(', '(', ')', '|', '!', '@', '+', '\\', '\\*', 'a', 'a', 'b', 'c', 'x', 'y', 'ab',
  'md', 'js', '.md', '.js', 'src', 'lib', 'deep', 'docs', 'foo', 'bar', '-', '_', ' ', '[[:alpha:]]', '[[:digit:]]', '}', '{', ',', 'é', '日',
  '**/', '*/', '/**', '../', './', '.*', '*.md', '*.{md,txt}', '!(*.md)', '+(*.js)', '{src,docs}', 'file', '1', '2', '10',
]
const fuzz = (n) => {
  const parts = []
  for (let i = 0; i < n; i++) parts.push(pick(tokens))
  return parts.join('')
}

// A pattern that is near to a real path of the tree, so that it matches something.
const mutate = (entry) => {
  const segs = entry.replace(/\/$/, '').split('/')
  const i = Math.floor(rnd() * segs.length)
  const s = segs[i]
  const chars = [...s]
  const c = Math.floor(rnd() * chars.length)
  const dot = s.lastIndexOf('.')
  const base = dot > 0 ? s.slice(0, dot) : s
  const ext = dot > 0 ? s.slice(dot) : ''
  const forms = [
    () => `*${ext || chars[chars.length - 1]}`,
    () => `${chars.slice(0, c).join('')}*`,
    () => `${chars.slice(0, c).join('')}?${chars.slice(c + 1).join('')}`,
    () => `${chars.slice(0, c).join('')}[${chars[c]}${pick(['x', 'z', '-z', 'a-z'])}]${chars.slice(c + 1).join('')}`,
    () => `${chars.slice(0, c).join('')}[!${chars[c]}]${chars.slice(c + 1).join('')}`,
    () => `{${s},${pick(['zz', 'q', base + 'x'])}}`,
    () => `+(${s}|${pick(['zz', base]) })`,
    () => `@(${base})${ext}`,
    () => `!(${base})${ext}`,
    () => `!(${s})`,
    () => `${base}?(${pick(['x', ext.slice(1) || 'y', 'md'])})${ext}`,
    () => `*(${base}|x)${ext}`,
    () => `${base}.@(${ext.slice(1) || 'md'}|js)`,
    () => `*${chars.slice(c).join('')}`,
    () => `${base}.{md,txt,js}`,
    () => `*.!(${ext.slice(1) || 'md'})`,
    () => `**`,
    () => `*`,
    () => `**${s}`,
    () => `.${s}`,
    () => `\\${s}`,
    () => `${s}/`,
    () => `./${s}`,
    () => `${chars.slice(0, c).join('')}*${chars.slice(c + 1).join('')}`,
    () => `+(${chars[c]}|${chars[(c + 1) % chars.length]})`,
  ]
  const f = pick(forms)()
  const segsOut = segs.slice()
  segsOut[i] = f
  let sub = segsOut
  const shape = rnd()
  if (shape < 0.2) sub = [...segsOut.slice(0, i), '**', ...segsOut.slice(i)]
  else if (shape < 0.3) sub = segsOut.map((x, j) => (j < i ? '*' : x))
  else if (shape < 0.4) sub = [...segsOut.slice(0, i + 1), '**', segsOut[segsOut.length - 1]]
  else if (shape < 0.45) sub = ['**', ...segsOut.slice(i)]
  return sub.join('/')
}

// ---------------------------------------------------------------- run
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'lexi-glob-'))
const build = (name) => {
  const wrap = path.join(tmp, name)
  const dir = path.join(wrap, 'root')
  fs.mkdirSync(dir, {recursive: true})
  for (const e of trees[name]) {
    if (typeof e === 'object') {
      fs.mkdirSync(path.dirname(path.join(dir, e.link)), {recursive: true})
      fs.symlinkSync(e.to.replace('__ROOT__', dir), path.join(dir, e.link))
    } else if (e.endsWith('/')) {
      fs.mkdirSync(path.join(dir, e), {recursive: true})
    } else {
      fs.mkdirSync(path.dirname(path.join(dir, e)), {recursive: true})
      fs.writeFileSync(path.join(dir, e), 'x')
    }
  }
  return dir
}

const hash = (s) => crypto.createHash('sha1').update(s).digest('hex').slice(0, 12)
// alex can throw from a callback, for a path that the ignore package rejects. That is an
// uncaught exception, so the promise of the call also settles on it.
let settle = null
process.on('uncaughtException', (error) => (settle ? settle({error}) : Promise.reject(error)))
const run = (fn, ...args) =>
  new Promise((resolve) => {
    settle = resolve
    fn(...args, (error, v) => resolve({error, v}))
  })
const swap = (s, dir) => s.split(dir).join('${ROOT}')

const kinds = {}
const crashes = new Set()
const seen = new Set()
let total = 0

for (const name of Object.keys(trees)) {
  const dir = build(name)
  console.error(name)
  const entries = trees[name].map((e) => (typeof e === 'object' ? e.link : e))
  const list = []
  const add = (kind, pattern) => {
    // A pattern from the root of the real file system would walk all of it.
    // Two levels up from the tree is the temporary directory: leave it out too.
    if (minimatch.braceExpand(pattern).some((x) => x.startsWith('/') || /(^|\/)\.\.\/+\.\.(\/|$)/.test(x))) return
    const key = `${name}\0${pattern}`
    if (!seen.has(key)) {
      seen.add(key)
      list.push([kind, pattern])
    }
  }
  for (const [kind, ps] of Object.entries(fixed)) for (const p of ps) add(kind, p)
  for (const e of entries) {
    add('literal', e)
    add('literal', `./${e}`)
  }
  for (let i = 0; i < 260; i++) add('near', mutate(pick(entries)))
  for (let i = 0; i < 130; i++) add('fuzz', fuzz(1 + Math.floor(rnd() * 7)))
  for (let i = 0; i < 40; i++) add('absolute', '${ROOT}/' + mutate(pick(entries)))
  for (const p of ['${ROOT}', '${ROOT}/', '${ROOT}/*', '${ROOT}/**', '${ROOT}/*/', '${ROOT}/.*', '${ROOT}/**/*.md']) add('absolute', p)

  for (const [kind, p] of list) {
    const pattern = p.split('${ROOT}').join(dir)
    const t0 = Date.now()
    if (process.env.GLOB_TRACE) console.error(JSON.stringify(p))
    let magic
    try {
      magic = glob.hasMagic(pattern)
    } catch (error) {
      crashes.add(JSON.stringify(p)) // alex stops with a TypeError: not a case
      continue
    }
    const g = await run(glob, pattern, {cwd: dir})
    // The finder walks a directory without a check for cycles, so a symbolic link to a parent
    // makes alex run for ever. The finder runs only when no argument or match is a directory.
    const cycles = name === 'links' && (magic ? g.v : [pattern]).some((x) => fs.statSync(path.resolve(dir, x), {throwIfNoEntry: false})?.isDirectory())
    const f = cycles ? {v: {files: []}} : await run(finder, [pattern], {cwd: dir, extensions: ['.md', '.txt'], ignorePatterns: [], silentlyIgnore: false, ignore: {check: (fp, cb) => cb(null, false)}})
    if (g.error) throw new Error(`${name} ${JSON.stringify(p)}: ${g.error}`)
    // The finder crashed: alex stops with an error, so there is no list of files to compare.
    const crash = f.error ? String(f.error.message).slice(0, 80) : undefined
    const reported = (f.error ? [] : f.v.files).map((file) => {
      const m = file.messages.length ? ` !${file.messages[0].reason}` : ''
      return swap(file.path, dir) + m
    })
    // The finder reads in the order of the file system, and its reads end in any order: record the set.
    const files = [...new Set(reported)].sort()
    if (process.env.GLOB_TRACE && Date.now() - t0 > 300) console.error(`SLOW ${Date.now() - t0}ms ${JSON.stringify(p)} rss=${Math.round(process.memoryUsage().rss / 1e6)}MB`)
    ;(kinds[kind] ||= []).push({
      id: hash(`${name}\0${p}`),
      tree: name,
      kind,
      pattern: p,
      magic,
      want: g.v.map((x) => swap(x, dir)),
      ...(cycles ? {nofiles: true} : crash ? {crash} : {files}),
    })
    total++
  }
}

fs.rmSync(out, {recursive: true, force: true})
fs.mkdirSync(out, {recursive: true})
const stable = (v) => JSON.stringify(v, null, 1).replace(/\n\s*/g, ' ')
fs.writeFileSync(path.join(out, 'trees.json'), '{\n' + Object.entries(trees).map(([k, v]) => ` ${JSON.stringify(k)}: ${stable(v)}`).join(',\n') + '\n}\n')
for (const [kind, cases] of Object.entries(kinds)) {
  fs.writeFileSync(path.join(out, `${kind}.jsonl`), cases.map((c) => JSON.stringify(c)).join('\n') + '\n')
}
fs.rmSync(tmp, {recursive: true, force: true})
console.log(`patterns where glob.hasMagic throws, left out: ${[...crashes].join(' ')}`)
console.log(`${total} cases in ${Object.keys(kinds).length} files`)
