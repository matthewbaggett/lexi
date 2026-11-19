git init -q -b main .
git add -A
git commit -q -m one
printf '<!--alex frob-->\n' >> a.md

git add -A
git commit -q -m two
