git init -q -b main .
git add -A
git commit -q -m one
git checkout -q -b topic
printf 'He topic.\n' >> a.md
git commit -q -am topic
git checkout -q main
printf 'He main.\n' >> b.md
git commit -q -am main
git merge -q --no-ff -m merge topic
