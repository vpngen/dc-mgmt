## DEB-package automation

`./cmd` - директория с исходным кодом `.go` и `.sh`.
 - `.go` - компилируем бинарники с именем папки. Складываем в `${TMP_DIR}` 
 - `.sh` - копируем в `${TMP_DIR}`


Сборка DEB-пакета осуществляется nsfp-docker.


## TODO
- [x] выборка версии Go из go.mod
- [x] компиляция всех main.go c параметрами из сребы окружения
- [] обработка бинарников в шел-скриптов nfpm
- [] настройка preinstall-nfpm скрипта


## Hints

### Calculated IPv6 to IPv4:

```shell
echo "fdcc:1786:d861::3" | cut -f 2,3 -d ':' | sed 's/\://' | xxd -r -p | hexdump -v -e '/1 "%u."' | sed 's/\.$/\n/'
```

### Keydesk brigade ID to Database UUID:

```shell
echo "${brigade_id}=========" | base32 -d 2>/dev/null | hexdump -ve '1/1 "%02x"'
```

### Database UUID to Keydesk brigade ID:

```shell
echo "${brigade_id}" | xxd -r -p -l 16 | base32 | tr -d "="
```

