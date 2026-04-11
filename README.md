# install-node-modules

![bench chart](./chart.png)

## Bench

first run `./build.sh <name>`
then run `./run.sh <number of megs>`

Minimal requirements:

0. **she/it GO (ours) - 168m RAM**
1. **she/it (ours) - 192m RAM**
2. bun - 298m RAM
3. yarn - 325m RAM
4. pnpm - 449m RAM
5. npm - 482m RAM
6. flashinstall - 504m RAM

PS: i tested this on 4vcpu 8G VPS, so this is clean environment without any interference with the results.

## Other installers

[click](https://github.com/conaticus/click) did not work

[vold](https://github.com/suptejas/volt) support ended, did not work

[ied](https://github.com/alexanderGugel/ied) does not work with modern packages?

[caladan](https://github.com/healeycodes/caladan) would not compile
(actually has a good write-up https://healeycodes.com/installing-npm-packages-very-quickly)

[boltpm](https://github.com/nom-nom-hub/boltpm) fetches localhost???

