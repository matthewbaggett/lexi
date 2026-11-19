git init -q -b main .
git add -A
git commit -q -m one
printf 'He new.\n' >> s/a.md
printf 'He new.\n' >> b.md

git add -A
git commit -q -m two
