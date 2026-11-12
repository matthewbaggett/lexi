# Known differences from alex

The corpus holds 17205 cases. lexi and alex 11.0.1 agree on all of them, so no case is an expected
diff. `expectedDiffs` in `internal/rules/corpus_test.go` is empty.

When a difference needs a design decision, list it here with the case id and the reason. Then add
the id to `expectedDiffs`. The test fails when an id in that map is not in the corpus.
