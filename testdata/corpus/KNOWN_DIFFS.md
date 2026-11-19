# Known differences from alex

The language corpus (`equality` and `profanity`) holds 17205 cases. lexi and alex 11.0.1 agree on all
of them, so no case there is an expected diff. `expectedDiffs` in `internal/rules/corpus_test.go` is
empty.

The format corpus (`markdown`, `html` and `mdx`) marks each expected diff with a `known` field. Its
value is the name of a section below. `TestFormatCorpus` in `internal/input` checks three things:
the section exists, the document still differs, and every other document agrees with alex. A
document that lexi now reads like alex fails the test, so fix the mark when you fix lexi.

## mdx-syntax-error

alex reads an MDX file with a real JavaScript parser (acorn). It stops with an error for a file that
is not valid MDX. lexi reports the errors that need no JavaScript parser, with the text of alex: a
bad character in a JSX tag, an expression or a tag that is not closed, an empty attribute
expression, and a closing tag that does not match. It stops there and reports no finding for the file.

lexi does not report these errors. It reads the file and reports its findings:

- An error of acorn. This is a bad JavaScript expression (`{1 +}`, `{...a b}`), or a bad `import` or
  `export` block. The text of the error is the text of acorn.
- An error that needs a syntax tree: an `export` block with a statement that is not `import` or
  `export`, and an attribute expression that is not a spread (`<A {a} />`).
- A tag that is open when an emphasis ends (`*a <B>b* c</B>`).
- A line that ends with a lone `\r`.

When one of these comes before another error in the file, lexi reports that other error, and alex
reports the first.

State: partial. The text of acorn needs a JavaScript parser, and no Go library gives that text. A
parser would add up to 5.9 MB to the binary, and it would catch only broken files, which the build of
the MDX site reports anyway. See [docs/Investigations/MDX.md](../../docs/Investigations/MDX.md).

## markdown-lone-cr

A file with a lone `\r` as a line ending, and a tab that indents its first line, is an indented code
block in goldmark. goldmark does not end the line at the lone `\r`, so the code block swallows the
next line. micromark does end the line.

State: not planned. Lone `\r` line endings are from classic Mac OS. The other documents in the corpus
with lone `\r` line endings agree.

## micromark-container-end

micromark ends some container nodes (a list item in a footnote, a list after a block quote with a
blank line) at the start of the next block, on the next line. `mdast-util-to-nlcst` then sees no line
break between the two blocks, and it joins the last word of the first block with the first word of
the next block. alex does not report the joined word. lexi puts a paragraph break between the blocks
and reports it.

State: not planned. The cause is a position quirk of one parser in one rare shape of document.

## markdown-autolink-literal

remark-gfm turns a bare `http://` address into a link node, and that node splits the text around it.
The position of a finding after the address then comes from the start of a new text node. goldmark
has no such extension in lexi, so the text stays one node. After a character reference in the same
paragraph, the columns of the two tools differ by the length of the reference.

State: not planned. The case needs a bare address and a character reference in one paragraph. The
finding is right, and only its column is off.

## markdown-task-link

goldmark reads `[x](y)` at the start of a list item as a task list check box and the text `(y)`.
remark-gfm reads a link, because a check box needs a space after the bracket. The words of the
link and the text after it are one word in alex, and lexi sees only the text.

State: not planned. A list item that starts with a link whose text is one `x`, `X` or space is rare.
