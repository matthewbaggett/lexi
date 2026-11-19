// Builds testdata/corpus/mdx/adversarial.jsonl: MDX documents that try to break the blanker of
// internal/input/mdx.go. It records what the real alex 11.0.1 reports for each one. For an
// error it also records `at`, the "line:column" that alex gives, and `rule`, "source:ruleId".
//
//   ALEX_ORACLE=<dir with alex@11.0.1> node scripts/corpus/generate-mdx-adversarial.mjs
//
// The row format is the one of generate-formats.mjs, with the extra fields `at` and `rule`.
// A document where lexi differs from alex has a `known` field that names a section of
// KNOWN_DIFFS.md. DIFF_FILE=<list of differences> sets `known` for you. See below.
import crypto from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import {createRequire} from 'node:module'
import {pathToFileURL} from 'node:url'

const oracle = process.env.ALEX_ORACLE
if (!oracle) throw new Error('set ALEX_ORACLE to a folder where alex@11.0.1 is installed')
const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..')
const alex = await import(pathToFileURL(createRequire(path.join(oracle, 'x.js')).resolve('alex')).href)

const TERMS = ['blacklist', 'whitelist', 'chairman', 'handicapped', 'crazy', 'policeman', 'fireman', 'mankind', 'postman', 'damn', 'crap', 'hell', 'dumb']
let tick = 0
const term = () => TERMS[tick++ % TERMS.length]
const docs = []
const add = (kind, label, text) => docs.push({kind, label, text})

// ---- Fragments of MDX. Each one holds a term (`T`) in prose, so that a wrong blank shows.
const T = '@@'
const inline = [
  ['expr-string-brace', `A {"}"} ${T} here.`],
  ['expr-string-brace2', `A {'{'} ${T} here.`],
  ['expr-template', 'A {`a ${b} }`} ' + T + ' here.'],
  ['expr-template-nested', 'A {`a ${`b ${c}`} }`} ' + T + ' here.'],
  ['expr-object', `A {{a: 1, b: {c: 2}}.a} ${T} here.`],
  ['expr-arrow', `A {x => { return x }} ${T} here.`],
  ['expr-line-comment', `A {a // }\n} ${T} here.`],
  ['expr-block-comment', `A {a /* } */} ${T} here.`],
  ['expr-regex', `A {/}/.test(a)} ${T} here.`],
  ['expr-regex-slash', `A {a / 2 / 3} ${T} here.`],
  ['expr-empty', `A {} ${T} here.`],
  ['expr-comment-only', `A {/* note */} ${T} here.`],
  ['expr-multiline', `A {a +\n  b} ${T} here.`],
  ['expr-multiline-blank', `A {a +\n\n  b} ${T} here.`],
  ['expr-escaped-quote', `A {"a\\"}"} ${T} here.`],
  ['expr-unicode', `A {"é😀"} ${T} here.`],
  ['expr-jsx-inside', `A {<b>x</b>} ${T} here.`],
  ['expr-jsx-gt', `A {a > b} ${T} here.`],
  ['expr-spread-text', `${T} {...a} here.`],
  ['expr-two', `{a} ${T} {b}`],
  ['expr-adjacent', `{a}{b}${T}{c}`],
  ['expr-backtick-outside', 'A `{` then ' + T + ' then `}`.'],
  ['expr-code-span-brace', 'A `{a}` and {b} ' + T + '.'],
  ['expr-escaped-brace', `A \\{ ${T} \\} here.`],
  ['expr-entity', `A &#123; ${T} &#125; here.`],
  ['jsx-self', `A <Foo /> ${T} here.`],
  ['jsx-pair', `A <Foo>${T}</Foo> here.`],
  ['jsx-nested', `A <A><B><C>${T}</C></B></A> here.`],
  ['jsx-nested-attrs', `A <A x="1"><B y={2}><C z='3'>${T}</C></B></A> here.`],
  ['jsx-attr-gt', `A <Foo a=">" /> ${T} here.`],
  ['jsx-attr-expr', `A <Foo a={1 > 2} /> ${T} here.`],
  ['jsx-attr-expr-str', `A <Foo a={"}"} /> ${T} here.`],
  ['jsx-attr-expr-tpl', 'A <Foo a={`${x}}`} /> ' + T + ' here.'],
  ['jsx-attr-expr-obj', `A <Foo a={{b: {c: 1}}} /> ${T} here.`],
  ['jsx-attr-spread', `A <Foo {...a} /> ${T} here.`],
  ['jsx-attr-spread-two', `A <Foo {...a} {...b} c /> ${T} here.`],
  ['jsx-attr-multiline', `A <Foo\n  a="1"\n  b={2}\n/> ${T} here.`],
  ['jsx-attr-multiline-blank', `A <Foo\n\n  a="1" /> ${T} here.`],
  ['jsx-attr-str-newline', `A <Foo a="x\ny" /> ${T} here.`],
  ['jsx-attr-ns', `A <Foo xlink:href="x" /> ${T} here.`],
  ['jsx-attr-text-term', `A <Foo title="${T}" /> then ${T} here.`],
  ['jsx-member', `A <Foo.Bar>${T}</Foo.Bar> here.`],
  ['jsx-ns', `A <svg:rect>${T}</svg:rect> here.`],
  ['jsx-lower', `A <div>${T}</div> here.`],
  ['jsx-fragment', `A <>${T}</> here.`],
  ['jsx-fragment-nested', `A <><>${T}</></> here.`],
  ['jsx-fragment-attr', `A <></> ${T} <Foo /> here.`],
  ['jsx-close-only', `A </Foo> ${T} here.`],
  ['jsx-text-inside-nested', `A <A>one <B>${T}</B> two</A> three.`],
  ['jsx-expr-child', `A <A>{b}${T}{c}</A> here.`],
  ['jsx-expr-child-brace', `A <A>{"}"} ${T}</A> here.`],
  ['jsx-emph', `A <A>*${T}*</A> and **${T}**.`],
  ['jsx-link', `A <A>[${T}](http://x.y)</A> here.`],
  ['jsx-code', 'A <A>`' + T + '`</A> here.'],
  ['jsx-entity', `A <A>&amp; ${T}</A> here.`],
  ['jsx-gt-text', `1 > 0 and ${T}.`],
  ['jsx-lt-space', `1 < 2 and ${T}.`],
  ['jsx-lt-digit', `a <3 ${T}.`],
  ['jsx-autolink', `<http://x.y> ${T}.`],
  ['jsx-autolink-mail', `<a@b.c> ${T}.`],
  ['jsx-dollar-name', `A <$a>${T}</$a> here.`],
  ['jsx-underscore-name', `A <_a>${T}</_a> here.`],
  ['jsx-hyphen-name', `A <a-b>${T}</a-b> here.`],
  ['jsx-unicode-name', `A <é>${T}</é> here.`],
  ['jsx-comment-in-tag', `A <Foo /* c */ a="1" /> ${T} here.`],
  ['jsx-attr-single-quote', `A <Foo a='"' /> ${T} here.`],
  ['jsx-attr-entity', `A <Foo a="&amp;" /> ${T} here.`],
  ['jsx-attr-no-value', `A <Foo a b c /> ${T} here.`],
]
const flow = [
  ['flow-jsx-self', `<Foo />\n\n${T}`],
  ['flow-jsx-pair', `<Foo>\n${T}\n</Foo>`],
  ['flow-jsx-pair-blank', `<Foo>\n\n${T}\n\n</Foo>`],
  ['flow-jsx-pair-md', `<Foo>\n\n# ${T}\n\n- ${T}\n\n</Foo>`],
  ['flow-jsx-nested-blank', `<A>\n\n<B>\n\n${T}\n\n</B>\n\n</A>`],
  ['flow-jsx-nested-tight', `<A>\n<B>\n${T}\n</B>\n</A>`],
  ['flow-jsx-indented', `<A>\n  <B>\n    ${T}\n  </B>\n</A>`],
  ['flow-jsx-indented-blank', `<A>\n\n  <B>\n\n    ${T}\n\n  </B>\n\n</A>`],
  ['flow-jsx-text-after', `<Foo /> ${T}`],
  ['flow-jsx-text-before', `${T} <Foo />`],
  ['flow-jsx-two-same-line', `<A /><B />\n\n${T}`],
  ['flow-jsx-two-text', `<A /> ${T} <B />`],
  ['flow-jsx-multiline-attr', `<Foo\n  a="1"\n  b={{c: 1}}\n/>\n\n${T}`],
  ['flow-jsx-multiline-attr-text', `<Foo\n  a="1"\n/> ${T}`],
  ['flow-jsx-fragment', `<>\n\n${T}\n\n</>`],
  ['flow-jsx-fragment-tight', `<>\n${T}\n</>`],
  ['flow-jsx-fragment-inline', `<>${T}</>`],
  ['flow-jsx-fragment-nested', `<>\n<>\n${T}\n</>\n</>`],
  ['flow-jsx-self-closing-tight', `<Foo/>\n${T}`],
  ['flow-expr', `{a}\n\n${T}`],
  ['flow-expr-multiline', `{\n  a +\n  b\n}\n\n${T}`],
  ['flow-expr-text-after', `{a} ${T}`],
  ['flow-expr-text-before', `${T} {a}`],
  ['flow-expr-comment', `{/* c */}\n\n${T}`],
  ['flow-expr-comment-multiline', `{/*\n  c\n*/}\n\n${T}`],
  ['flow-expr-string-brace', `{"}"}\n\n${T}`],
  ['flow-expr-template', '{`}`}\n\n' + T],
  ['flow-expr-template-multiline', '{`a\n\nb`}\n\n' + T],
  ['flow-expr-obj', `{{a: {b: 1}}.a}\n\n${T}`],
  ['flow-expr-fn', `{[1, 2].map((x) => {\n  return x\n})}\n\n${T}`],
  ['flow-expr-jsx', `{[1].map((x) => (\n  <li key={x}>{x}</li>\n))}\n\n${T}`],
  ['flow-expr-jsx-text', `{x && <p>${T}</p>}\n\n${T}`],
  ['flow-expr-two', `{a}\n{b}\n\n${T}`],
  ['flow-expr-indent', `  {a}\n\n${T}`],
  ['flow-expr-code-indent', `    {a}\n\n${T}`],
  ['flow-esm-import', `import A from 'a'\n\n${T}`],
  ['flow-esm-import-multi', `import A from 'a'\nimport {B, C} from "b"\n\n${T}`],
  ['flow-esm-import-brace-multiline', `import {\n  A,\n  B,\n} from 'a'\n\n${T}`],
  ['flow-esm-import-star', `import * as A from 'a'\n\n${T}`],
  ['flow-esm-import-side', `import 'a'\n\n${T}`],
  ['flow-esm-export-const', `export const a = 1\n\n${T}`],
  ['flow-esm-export-default', `export default function Layout({children}) {\n  return <div>{children}</div>\n}\n\n${T}`],
  ['flow-esm-export-obj', `export const meta = {\n  title: 'x',\n  tags: ['a', 'b'],\n}\n\n${T}`],
  ['flow-esm-comment-line', `// a comment\nexport const a = 1\n\n${T}`],
  ['flow-esm-comment-in', `export const a = 1 // note }\n\n${T}`],
  ['flow-esm-comment-block', `export const a = /* } */ 1\n\n${T}`],
  ['flow-esm-comment-block-multi', `export const a = /*\n  blank follows:\n\n  still comment\n*/ 1\n\n${T}`],
  ['flow-esm-template', 'export const a = `x ${1 + 1} y`\n\n' + T],
  ['flow-esm-template-multiline', 'export const a = `x\ny`\n\n' + T],
  ['flow-esm-template-blank', 'export const a = `x\n\ny`\n\n' + T],
  ['flow-esm-template-nested', 'export const a = `x ${`y ${z}`} w`\n\n' + T],
  ['flow-esm-string-brace', `export const a = "}"\n\n${T}`],
  ['flow-esm-regex', `export const a = /}/\n\n${T}`],
  ['flow-esm-jsx', `export const A = () => <b>x</b>\n\n${T}`],
  ['flow-esm-jsx-multiline', `export const A = () => (\n  <div>\n    <b>x</b>\n  </div>\n)\n\n${T}`],
  ['flow-esm-no-blank', `export const a = 1\n${T}`],
  ['flow-esm-then-md', `export const a = 1\n\n# ${T}\n\n${T}`],
  ['flow-esm-after-text', `${T}\nexport const a = 1`],
  ['flow-esm-after-text-blank', `${T}\n\nexport const a = 1\n\n${T}`],
  ['flow-esm-indented', `  export const a = 1\n\n${T}`],
  ['flow-esm-word', `exports are here ${T}`],
  ['flow-esm-word2', `export ${T}`],
  ['flow-esm-word3', `import ${T}`],
  ['flow-esm-importantly', `importantly ${T}`],
  ['flow-esm-semicolon', `export const a = 1;\nexport const b = 2;\n\n${T}`],
  ['flow-esm-class', `export class A {\n  b() {}\n}\n\n${T}`],
  ['flow-esm-async', `export async function a() {\n  await b()\n}\n\n${T}`],
  ['flow-esm-reexport', `export {a, b as c} from 'd'\n\n${T}`],
  ['flow-esm-reexport-star', `export * from 'd'\n\n${T}`],
  ['flow-esm-unicode', `export const é = "😀"\n\n${T}`],
  ['flow-esm-term-in-string', `export const a = "${T}"\n\n${T}`],
  ['flow-esm-term-in-comment', `// ${T}\nexport const a = 1\n\n${T}`],
  ['flow-esm-term-in-template', 'export const a = `' + T + '`\n\n' + T],
  ['flow-esm-crlf', `export const a = 1\r\n\r\n${T}`],
  ['flow-esm-fence-between', `export const a = 1\n\n\`\`\`js\nexport const b = ${T}\n\`\`\`\n\n${T}`],
  ['flow-fence-esm-like', `\`\`\`js\nexport const b = 1\n{${T}}\n<A>\n\`\`\`\n\n${T}`],
  ['flow-fence-tilde', `~~~\n<A>{${T}\n~~~\n\n${T}`],
  ['flow-fence-long', `\`\`\`\`\n\`\`\`\n<A>\n\`\`\`\`\n\n${T}`],
  ['flow-fence-indented-code', `    <A>{x\n\n${T}`],
  ['flow-fence-jsx-in-fence-info', `\`\`\`jsx {1,3}\n<A>\n\`\`\`\n\n${T}`],
  ['flow-html-entity-ref', `&lt;A&gt; ${T}`],
  ['flow-heading-jsx', `# <A>${T}</A>\n\n${T}`],
  ['flow-heading-expr', `# {a} ${T}\n\n${T}`],
  ['flow-heading-setext', `<A>${T}</A>\n=====\n\n${T}`],
  ['flow-thematic-jsx', `<A />\n***\n\n${T}`],
  ['flow-quote-jsx', `> <A>\n> ${T}\n> </A>`],
  ['flow-quote-jsx-blank', `> <A>\n>\n> ${T}\n>\n> </A>`],
  ['flow-quote-expr', `> {a}\n> ${T}`],
  ['flow-quote-esm', `> export const a = 1\n\n${T}`],
  ['flow-list-jsx', `- <A>${T}</A>\n- {b} ${T}\n- <C />`],
  ['flow-list-jsx-multi', `- <A>\n  ${T}\n  </A>\n- ${T}`],
  ['flow-list-jsx-blank', `- <A>\n\n  ${T}\n\n  </A>\n- ${T}`],
  ['flow-list-expr-multi', `- {a +\n  b} ${T}\n- ${T}`],
  ['flow-list-nested', `- a\n  - <B>${T}</B>\n    - {c} ${T}`],
  ['flow-list-ordered', `1. <A>${T}</A>\n2. {b}\n3. ${T}`],
  ['flow-list-attr-multi', `- <A\n    b="1"\n  /> ${T}\n- ${T}`],
  ['flow-list-fragment', `- <>${T}</>\n- <>\n  ${T}\n  </>`],
  ['flow-list-tabs', `-\t<A>${T}</A>\n-\t${T}`],
  ['flow-list-task', `- [ ] <A>${T}</A>\n- [x] {b} ${T}`],
  ['flow-list-expr-text', `* {a}\n* ${T}\n* {b}`],
  ['flow-table-jsx', `| a | b |\n|---|---|\n| <A>${T}</A> | ${T} |`],
  ['flow-table-expr', `| a | b |\n|---|---|\n| {x} | ${T} |`],
  ['flow-table-expr-pipe', `| a | b |\n|---|---|\n| {x || y} | ${T} |`],
  ['flow-table-expr-pipe-str', `| a | b |\n|---|---|\n| {"|"} | ${T} |`],
  ['flow-table-jsx-attr-pipe', `| a | b |\n|---|---|\n| <A b="|" /> | ${T} |`],
  ['flow-table-jsx-multi', `| a | b |\n|---|---|\n| <A> | ${T} |\n| </A> | x |`],
  ['flow-table-header-jsx', `| <A>${T}</A> | {b} |\n|---|---|\n| x | ${T} |`],
  ['flow-table-fragment', `| a |\n|---|\n| <>${T}</> |`],
  ['flow-table-code-brace', '| a | b |\n|---|---|\n| `{` | ' + T + ' |'],
  ['flow-table-esc-pipe', `| a | b |\n|---|---|\n| {x \\| y} | ${T} |`],
  ['flow-footnote-jsx', `A[^1] ${T}.\n\n[^1]: <A>${T}</A>`],
  ['flow-footnote-expr', `A[^1] ${T}.\n\n[^1]: {a} ${T}`],
  ['flow-def-jsx', `[a]: <b> "${T}"\n\n${T}`],
  ['flow-link-jsx', `[<A>${T}</A>](x) ${T}`],
  ['flow-link-expr', `[{a} ${T}](x)`],
  ['flow-link-url-brace', `[a](http://x/{y}) ${T}`],
  ['flow-link-url-lt', `[a](<http://x y>) ${T}`],
  ['flow-image-expr', `![{a}](x) ${T}`],
  ['flow-emph-expr', `*{a} ${T}* and _{b}_ and ${T}`],
  ['flow-emph-span', `*a <B>${T}* c</B> ${T}`],
  ['flow-hardbreak-expr', `{a}  \n${T}`],
  ['flow-hardbreak-jsx', `<A />  \n${T}`],
  ['flow-hardbreak-backslash', `{a}\\\n${T}`],
  ['flow-control-comment', `{/* alex ignore crazy */}\n\nA crazy ${T}.\n\nB crazy.`],
  ['flow-control-comment-inline', `A {/* alex ignore */} ${T} crazy.`],
  ['flow-control-comment-nested-jsx', `<A>\n\n{/* alex ignore */}\n\n${T}\n\n</A>`],
  ['flow-control-comment-list', `- {/* alex ignore */}\n- ${T}\n- ${T}`],
  ['flow-control-comment-disable', `{/* alex disable */}\n\n${T}\n\n{/* alex enable */}\n\n${T}`],
  ['flow-control-comment-quote', `> {/* alex ignore */}\n> ${T}\n\n${T}`],
  ['flow-control-comment-esm', `export const a = 1\n\n{/* alex ignore */}\n\n${T}`],
  ['flow-control-comment-multiline', `{/*\n  alex ignore\n*/}\n\n${T}`],
  ['flow-control-comment-jsx-after', `{/* alex ignore */}\n<A>${T}</A>\n\n${T}`],
  ['flow-control-comment-expr-after', `{/* alex ignore */}\n{a} ${T}\n\n${T}`],
  ['flow-frontmatter', `---\ntitle: ${T}\n---\n\n<A>${T}</A>`],
  ['flow-frontmatter-brace', `---\ntitle: "{"\n---\n\n${T}`],
  ['flow-blank-lines-ws', `<A>\n \t\n${T}\n  \n</A>`],
  ['flow-crlf-jsx', `<A>\r\n\r\n${T}\r\n\r\n</A>`],
  ['flow-crlf-expr', `{a +\r\n b}\r\n\r\n${T}`],
  ['flow-crlf-attr', `<A\r\n  b="1"\r\n/> ${T}`],
  ['flow-cr-expr', `{a +\r b} ${T}`],
  ['flow-tab-jsx', `\t<A>\n\t${T}\n\t</A>`],
  ['flow-bom', `﻿<A>${T}</A>\n\n${T}`],
  ['flow-nbsp', `<A> ${T}</A> ${T}`],
  ['flow-emoji-pos', `😀 {a} 😀 <B>x</B> ${T}`],
  ['flow-emoji-attr', `<A b="😀" c={"😀"} /> ${T}`],
  ['flow-cjk', `日本 <A>日本</A> {b} ${T}`],
  ['flow-rtl', `שלום <A>שלום</A> ${T}`],
  ['flow-combining', `é {a} ${T}`],
  ['flow-long-line', `${'word '.repeat(200)}{a} <B /> ${T}`],
]
// Syntax errors. alex stops on every one.
const errors = [
  ['err-unclosed-tag', `<Foo>\n\n${T}`],
  ['err-unclosed-inline', `A <Foo>${T} here.`],
  ['err-unclosed-nested', `<A>\n<B>\n${T}\n</A>`],
  ['err-extra-close', `${T}\n\n</Foo>`],
  ['err-mismatch', `<A>${T}</B>`],
  ['err-mismatch-case', `<a>${T}</A>`],
  ['err-mismatch-member', `<A.B>${T}</A.C>`],
  ['err-fragment-mismatch', `<>${T}</A>`],
  ['err-close-attr', `<A>${T}</A b>`],
  ['err-close-selfclose', `<A>${T}</A/>`],
  ['err-selfclose-open-close', `</A/>`],
  ['err-tag-eof', `${T} <Foo`],
  ['err-tag-eof-attr', `${T} <Foo a=`],
  ['err-tag-eof-str', `${T} <Foo a="x`],
  ['err-tag-eof-expr', `${T} <Foo a={`],
  ['err-tag-name-char', `A <1> ${T}`],
  ['err-tag-name-bang', `<!-- ${T} -->`],
  ['err-tag-name-bang-inline', `A <!-- ${T} --> b`],
  ['err-tag-doctype', `<!doctype html>\n\n${T}`],
  ['err-tag-cdata', `<![CDATA[x]]>\n\n${T}`],
  ['err-tag-pi', `<?php ?>\n\n${T}`],
  ['err-tag-name-dash-start', `A <-a> ${T}`],
  ['err-tag-name-dot', `A <a.> ${T}`],
  ['err-tag-name-dot-start', `A <.a> ${T}`],
  ['err-tag-name-colon-member', `A <a:b.c> ${T}`],
  ['err-tag-name-member-colon', `A <a.b:c> ${T}`],
  ['err-tag-ns-two', `A <a:b:c> ${T}`],
  ['err-tag-attr-unquoted', `<A b=c /> ${T}`],
  ['err-tag-attr-bad-name', `<A 1="x" /> ${T}`],
  ['err-tag-attr-dup-ok', `<A b="1" b="2" /> ${T}`],
  ['err-tag-attr-after-slash', `<A / b> ${T}`],
  ['err-tag-slash-text', `<A /x> ${T}`],
  ['err-tag-slash-gt-space', `<A / > ${T}`],
  ['err-tag-gt-in-name', `<A>>${T}`],
  ['err-tag-lt-in-tag', `<A <B> /> ${T}`],
  ['err-tag-attr-tag', `<A b=<C /> /> ${T}`],
  ['err-tag-equal-no-name', `<A ="x" /> ${T}`],
  ['err-tag-brace-close-first', `<A }> ${T}`],
  ['err-tag-close-slash-twice', `</A> </A> ${T}`],
  ['err-tag-lazy', `> <A>\n${T}`],
  ['err-tag-lazy-close', `> <A>\n> ${T}\n</A>`],
  ['err-tag-list-close', `- <A>\n- ${T}\n- </A>`],
  ['err-tag-list-indent', `- <A>\n\n  ${T}\n\n</A>`],
  ['err-tag-table-split', `| a |\n|---|\n| <A> |\n\n${T}\n\n| </A> |`],
  ['err-expr-unclosed', `A {a ${T}`],
  ['err-expr-unclosed-flow', `{a\n\n${T}`],
  ['err-expr-close-first', `A } ${T}`],
  ['err-expr-close-first-flow', `}\n\n${T}`],
  ['err-expr-bad-js', `A {1 +} ${T}`],
  ['err-expr-bad-js2', `A {a b} ${T}`],
  ['err-expr-bad-js-flow', `{a b}\n\n${T}`],
  ['err-expr-bad-js-multi', `{a +\n  +\n}\n\n${T}`],
  ['err-expr-bad-js-late', `{a\n\n\n\n+}\n\n${T}`],
  ['err-expr-bad-str', `A {"a} ${T}`],
  ['err-expr-bad-str-nl', `A {"a\nb"} ${T}`],
  ['err-expr-bad-tpl', 'A {`a} ' + T],
  ['err-expr-bad-regex', `A {/[/} ${T}`],
  ['err-expr-statement', `A {var a = 1} ${T}`],
  ['err-expr-two-exprs', `A {a; b} ${T}`],
  ['err-expr-comma', `A {a, b} ${T}`],
  ['err-expr-spread-text', `A {...a} ${T}`],
  ['err-expr-arrow-block', `A {() => {}} ${T}`],
  ['err-expr-await', `A {await a} ${T}`],
  ['err-expr-yield', `A {yield a} ${T}`],
  ['err-expr-import', `A {import('a')} ${T}`],
  ['err-expr-import-meta', `A {import.meta.url} ${T}`],
  ['err-expr-new-target', `A {new.target} ${T}`],
  ['err-expr-return', `A {return a} ${T}`],
  ['err-expr-jsx-unclosed', `A {<b>} ${T}`],
  ['err-expr-jsx-bad', `A {<b></c>} ${T}`],
  ['err-expr-jsx-comment', `A {<!-- c -->} ${T}`],
  ['err-expr-jsx-two', `A {<a /><b />} ${T}`],
  ['err-expr-jsx-attr-bad', `A {<a b=c />} ${T}`],
  ['err-expr-nested-unclosed', `A {{a: 1} ${T}`],
  ['err-expr-in-code-ok', 'A `{a` ' + T],
  ['err-expr-in-tag-attr-bad', `<A b={1 +} /> ${T}`],
  ['err-expr-in-tag-attr-unclosed', `<A b={1 /> ${T}`],
  ['err-expr-spread-attr-bad', `<A {a} /> ${T}`],
  ['err-expr-spread-attr-two', `<A {...a, ...b} /> ${T}`],
  ['err-expr-spread-attr-empty', `<A {...} /> ${T}`],
  ['err-expr-spread-attr-comment', `<A {/* c */} /> ${T}`],
  ['err-expr-spread-attr-empty-braces', `<A {} /> ${T}`],
  ['err-expr-attr-empty', `<A b={} /> ${T}`],
  ['err-expr-attr-comment', `<A b={/* c */} /> ${T}`],
  ['err-expr-attr-multi-ok', `<A b={1,\n 2} /> ${T}`],
  ['err-expr-quote-heading', `# {a ${T}`],
  ['err-expr-list', `- {a\n- ${T}`],
  ['err-expr-quote-lazy', `> {a\n${T}}`],
  ['err-expr-quote-marker', `> {a\n> b} ${T}`],
  ['err-expr-quote-marker-inside', `> {a\n>\n> b} ${T}`],
  ['err-expr-list-cont', `- {a\n  b} ${T}`],
  ['err-expr-list-cont-bad', `- {a\nb} ${T}`],
  ['err-expr-table-cell', `| a |\n|---|\n| {b |\n\n${T}`],
  ['err-expr-after-fence', `\`\`\`\n{\n\`\`\`\n\n{a ${T}`],
  ['err-esm-bad', `import x from\n\n${T}`],
  ['err-esm-bad2', `export const = 1\n\n${T}`],
  ['err-esm-bad-braces', `export const a = {\n\n${T}`],
  ['err-esm-bad-blank-inside', `export const a = {\n  b: 1,\n\n  c: 2,\n}\n\n${T}`],
  ['err-esm-bad-tpl', 'export const a = `x\n\n' + T],
  ['err-esm-statement', `export default 1\nexport default 2\n\n${T}`],
  ['err-esm-non-esm', `import a from 'a'\nconsole.log(a)\n\n${T}`],
  ['err-esm-export-non-decl', `export 1\n\n${T}`],
  ['err-esm-import-dynamic', `import('a')\n\n${T}`],
  ['err-esm-import-meta', `import.meta.url\n\n${T}`],
  ['err-esm-import-expr', `import(a)\n\n${T}`],
  ['err-esm-import-eof', `import a from 'a'`],
  ['err-esm-import-semicolon-text', `import a from 'a'; ${T}\n\n${T}`],
  ['err-esm-jsx-unclosed', `export const A = () => <b>\n\n${T}`],
  ['err-esm-jsx-unclosed-multi', `export const A = () => (\n  <b>\n)\n\n${T}`],
  ['err-esm-jsx-mismatch', `export const A = <a></b>\n\n${T}`],
  ['err-esm-jsx-text-brace', `export const A = <a>}</a>\n\n${T}`],
  ['err-esm-jsx-comment', `export const A = <a><!-- c --></a>\n\n${T}`],
  ['err-esm-jsx-entity', `export const A = <a>&amp;</a>\n\n${T}`],
  ['err-esm-jsx-attr-str-nl', `export const A = <a b="x\ny" />\n\n${T}`],
  ['err-esm-unicode-id', `export const é = 1\n\n${T}`],
  ['err-esm-keyword-id', `export const class = 1\n\n${T}`],
  ['err-esm-strict', `export const a = 010\n\n${T}`],
  ['err-esm-with', `export const a = 1\nwith (a) {}\n\n${T}`],
  ['err-esm-reserved', `export const await = 1\n\n${T}`],
  ['err-esm-top-await', `export const a = await b\n\n${T}`],
  ['err-esm-dup-export', `export const a = 1\nexport const a = 2\n\n${T}`],
  ['err-esm-dup-import', `import a from 'a'\nimport a from 'b'\n\n${T}`],
  ['err-esm-label', `export const a = 1\nb: c\n\n${T}`],
  ['err-esm-quote-eof', `export const a = "\n\n${T}`],
  ['err-esm-comment-eof', `export const a = /*\n\n${T}`],
  ['err-esm-line-comment-brace', `export const a = {\n  // }\n}\n\n${T}`],
  ['err-esm-after-text-lazy', `${T}\nimport a from 'a'\n\n${T}`],
  ['err-late-position', `${'filler line\n\n'.repeat(40)}<Foo>\n\n${T}`],
  ['err-late-position-expr', `${'filler line\n\n'.repeat(40)}A {1 +} ${T}`],
  ['err-late-position-esm', `${'filler line\n\n'.repeat(40)}export const = 1\n\n${T}`],
  ['err-after-valid', `<A>${T}</A>\n\n{b}\n\nA <C>`],
  ['err-first-wins', `<A>\n\n{1 +}\n\n<B>`],
  ['err-first-wins2', `{1 +}\n\n<B>`],
  ['err-unicode-col', `😀 é <Foo`],
  ['err-unicode-col2', `😀 é {1 +}`],
  ['err-crlf-pos', `a\r\nb\r\n<Foo>\r\n${T}`],
  ['err-tab-col', `\t<Foo`],
  ['err-bom-col', `﻿<Foo`],
]

for (const [k, t] of inline) {
  // The sentence alone, after text, in a list item, in a quote, in a heading, and in a table cell.
  add(`inline-${k}`, 'positive', t)
  add(`inline-${k}`, 'positive', `Intro ${T} line.\n\n${t}\n\nOutro ${T}.`)
  add(`inline-${k}`, 'positive', `- one\n- ${t}\n- ${T}`)
  add(`inline-${k}`, 'positive', `> ${t.replace(/\n/g, '\n> ')}\n\n${T}`)
  add(`inline-${k}`, 'positive', `## ${t.replace(/\n/g, ' ')}\n\n${T}`)
  add(`inline-${k}`, 'positive', `| a | b |\n|---|---|\n| ${t.replace(/\n/g, ' ')} | ${T} |`)
}
for (const [k, t] of flow) {
  add(k, 'positive', t)
  add(k, 'positive', `Intro ${T} line.\n\n${t}`)
  add(k, 'positive', `${t}\n\nOutro ${T}.\n`)
  add(k, 'positive', `> ${t.replace(/\n/g, '\n> ')}`)
  add(k, 'positive', `- ${t.replace(/\n/g, '\n  ')}`)
}
for (const [k, t] of errors) {
  add(k, 'edge', t)
  add(k, 'edge', `Intro ${T} line.\n\n${t}`)
  add(k, 'edge', `- ${t.replace(/\n/g, '\n  ')}`)
}

// Terms inside code. alex stays silent about them. A blank line inside code is the hard part.
const code = [
  ['code-esm-blank-in-braces-comment', `export const a = {\n  b: 1,\n\n  // ${T}\n  c: 2,\n}\n\n${T}`],
  ['code-esm-blank-in-braces-string', `export const a = {\n  b: 1,\n\n  c: "${T}",\n}\n\n${T}`],
  ['code-esm-blank-in-parens', `export const A = () => (\n  <div>\n\n    ${T}\n\n  </div>\n)\n\n${T}`],
  ['code-esm-blank-in-jsx-text', `export const A = () => <div>\n\n  ${T}\n\n</div>\n\n${T}`],
  ['code-esm-blank-in-template', 'export const a = `x\n\n' + T + '`\n\n' + T],
  ['code-esm-blank-in-block-comment', `export const a = 1 /* x\n\n${T} */\n\n${T}`],
  ['code-esm-blank-in-import', `import {\n  a,\n\n  // ${T}\n  b,\n} from 'x'\n\n${T}`],
  ['code-esm-term-line-comment', `export const a = 1 // ${T}\n\n${T}`],
  ['code-esm-term-string-single', `export const a = '${T}'\n\n${T}`],
  ['code-esm-term-identifier', `export const ${T}s = 1\n\n${T}`],
  ['code-esm-term-key', `export const a = {${T}: 1}\n\n${T}`],
  ['code-esm-term-jsx-attr', `export const A = <b c="${T}" />\n\n${T}`],
  ['code-esm-term-jsx-text', `export const A = <b>${T}</b>\n\n${T}`],
  ['code-esm-term-regex', `export const a = /${T}/\n\n${T}`],
  ['code-esm-term-two-blocks', `export const a = 1\n\nexport const b = "${T}"\n\n${T}`],
  ['code-esm-term-after-prose', `${T}\n\nexport const b = "${T}"\n\n${T}`],
  ['code-expr-blank-comment', `{\n  // ${T}\n\n  a\n}\n\n${T}`],
  ['code-expr-blank-string-tpl', '{`x\n\n' + T + '`}\n\n' + T],
  ['code-expr-blank-block-comment', `{/*\n\n${T}\n\n*/}\n\n${T}`],
  ['code-expr-blank-object', `{{\n  a: 1,\n\n  b: "${T}",\n}.a}\n\n${T}`],
  ['code-expr-blank-array', `{[\n  1,\n\n  "${T}",\n].length}\n\n${T}`],
  ['code-expr-blank-jsx', `{x && (\n  <p>\n\n    ${T}\n\n  </p>\n)}\n\n${T}`],
  ['code-expr-term-string', `{"${T}"}\n\n${T}`],
  ['code-expr-term-key', `{{${T}: 1}.a}\n\n${T}`],
  ['code-expr-term-line-comment', `{a // ${T}\n}\n\n${T}`],
  ['code-expr-term-regex', `{/${T}/.test(a)}\n\n${T}`],
  ['code-expr-term-inline', `A {"${T}"} b ${T}.`],
  ['code-expr-term-inline-comment', `A {/* ${T} */} b ${T}.`],
  ['code-expr-term-inline-tpl', 'A {`${T}`} b ' + T + '.'],
  ['code-expr-term-inline-tpl-sub', 'A {`${"' + T + '"}`} b ' + T + '.'],
  ['code-expr-term-inline-multi', `A {a +\n  "${T}"} b ${T}.`],
  ['code-expr-term-inline-multi-comment', `A {a +\n  // ${T}\n  b} c ${T}.`],
  ['code-expr-term-list', `- {"${T}"}\n- ${T}`],
  ['code-expr-term-list-multi', `- {a +\n  "${T}"}\n- ${T}`],
  ['code-expr-term-quote-multi', `> {a +\n> "${T}"}\n\n${T}`],
  ['code-expr-term-quote-flow-multi', `> {\n>   // ${T}\n>   a\n> }\n\n${T}`],
  ['code-jsx-term-attr', `<A b="${T}">${T}</A>`],
  ['code-jsx-term-attr-multi', `<A\n  b="${T}"\n  c='${T}'\n/>\n\n${T}`],
  ['code-jsx-term-attr-expr', `<A b={"${T}"} />\n\n${T}`],
  ['code-jsx-term-attr-expr-comment', `<A b={a /* ${T} */} />\n\n${T}`],
  ['code-jsx-term-attr-expr-multi', `<A b={\n  // ${T}\n  a\n} />\n\n${T}`],
  ['code-jsx-term-attr-name', `<A ${T}="x" />\n\n${T}`],
  ['code-jsx-term-tag-name', `<${T[0]}${T}>x</${T[0]}${T}>`],
  ['code-jsx-term-attr-blank', `<A\n\n  b="${T}" />\n\n${T}`],
  ['code-jsx-term-attr-list', `- <A b="${T}" />\n- ${T}`],
  ['code-jsx-term-attr-quote-multi', `> <A\n>   b="${T}"\n> />\n\n${T}`],
  ['code-jsx-term-attr-table', `| a |\n|---|\n| <A b="${T}" /> |\n\n${T}`],
  ['code-jsx-term-spread', `<A {...${T}} />\n\n${T}`],
  ['code-jsx-term-member', `<A.${T[0]}${T}>x</A.${T[0]}${T}>`],
  ['code-fence-in-jsx', `<A>\n\n\`\`\`\n${T}\n\`\`\`\n\n</A>`],
  ['code-fence-in-esm-gap', `export const a = 1\n\n\`\`\`\n${T}\n\`\`\`\n\n${T}`],
  ['code-span-in-jsx', 'A <B>`' + T + '`</B> `{` ' + T + ' `}`.'],
  ['code-indented-in-list', `- a\n\n      ${T}\n\n- <B>${T}</B>`],
]
for (const [k, t] of code) {
  add(k, 'negative', t)
  add(k, 'negative', `Intro ${T} line.\n\n${t}`)
  add(k, 'negative', `- ${t.replace(/\n/g, '\n  ')}`)
  add(k, 'negative', `> ${t.replace(/\n/g, '\n> ')}`)
}

// ---- Run alex, record, and dedupe.
const alexRun = (text) => {
  try {
    const r = alex.mdx(text)
    const msgs = r.messages.map((x) => ({l: x.line, c: x.column, s: `${x.line}:${x.column}-${x.position.end.line}:${x.position.end.column}: ${x.reason} [${x.source}:${x.ruleId}]`}))
    return {want: msgs.map((v, i) => ({...v, i})).sort((p, q) => p.l - q.l || p.c - q.c || p.i - q.i).map((v) => v.s)}
  } catch (error) {
    return {want: [`ERROR ${error.message}`], at: `${error.line}:${error.column}`, rule: `${error.source}:${error.ruleId}`}
  }
}
const seen = new Set()
const rows = []
for (const d of docs) {
  const text = d.text.replaceAll(T, () => term())
  const id = crypto.createHash('sha256').update(`mdx\0${text}`).digest('hex').slice(0, 10)
  if (seen.has(id)) continue
  seen.add(id)
  const row = {id, category: 'mdx', kind: d.kind, label: d.label, text, ...alexRun(text)}
  rows.push(row)
}

// ---- Mark the documents where lexi differs, when asked. DIFF_FILE is the list that
// `CORPUS_DIFF_OUT=<file> go test ./internal/input -run FormatCorpus` writes. Run the generator
// once without it, run the test, then run the generator again with it.
if (process.env.DIFF_FILE) {
  const diff = fs.readFileSync(process.env.DIFF_FILE, 'utf8')
  for (const row of rows) {
    if (!diff.includes(`mdx/${row.kind} ${row.id}:`)) continue
    row.known = 'mdx-syntax-error'
  }
}

const file = path.join(root, 'testdata', 'corpus', 'mdx', 'adversarial.jsonl')
fs.writeFileSync(file, rows.map((r) => JSON.stringify(r)).join('\n') + '\n')
const errs = rows.filter((r) => r.want[0]?.startsWith('ERROR')).length
console.log(`wrote ${rows.length} documents (${errs} where alex stops with an error, ${rows.filter((r) => r.known).length} marked known)`)
