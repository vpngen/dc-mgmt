## DEB-package automation

`./cmd` - a directory containing the source code files with `.go` and `.sh` extensions.
 - `.go` - compile binaries with the same name as the directory. Store them in `${TMP_DIR}`.
 - `.sh` - copy them to `${TMP_DIR}`.


## TODO
- [x] Select Go version from go.mod
- [x] Compile all main.go files with environment variables
- [ ] Handle binaries in nfpm shell scripts
- [ ] Configure preinstall-nfpm script


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

## License

This project is licensed under the Mozilla Public License 2.0. See the [LICENSE](LICENSE) file for more details.

## Copyright

See the [COPYRIGHT](COPYRIGHT) file for detailed copyright information.
