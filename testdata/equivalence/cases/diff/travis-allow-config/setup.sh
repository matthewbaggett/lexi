git init -q -b main .
git add -A
git commit -q -m one
printf "\nHer new line.\n" >> a.md
printf 'He again.\n' >> a.md

git add -A
git commit -q -m two
