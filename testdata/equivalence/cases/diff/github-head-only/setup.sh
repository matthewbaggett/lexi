git init -q -b main .
git add -A
git commit -q -m one
git checkout -q -b feature
printf "\nHer new line.\n" >> a.md

git add -A
git commit -q -m two
