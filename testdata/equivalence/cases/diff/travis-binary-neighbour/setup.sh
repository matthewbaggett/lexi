git init -q -b main .
git add -A
git commit -q -m one
printf "\nHer new line.\n" >> a.md
printf '\000\001' > bin.dat

git add -A
git commit -q -m two
