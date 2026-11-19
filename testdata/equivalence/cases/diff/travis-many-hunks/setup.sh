git init -q -b main .
git add -A
git commit -q -m one
sed -i.bak -e 's/Line 3 fine/He 3/' -e 's/Line 30 fine/Her 30/' a.md
rm a.md.bak

git add -A
git commit -q -m two
