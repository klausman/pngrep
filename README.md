# PNGrep

A simple tool to grep through the text hunks of PNG images. Similar in use to
`exigrep`, which does the same for the EXIF tags of JPEG/JFIF files.

Usage of pngrep:

```
pngrep [options] <regex> <file> [file, ...]
Options:
  -i	Make regexp case-insensitive
  -w	Show matching text chunk
```

Differences to classic grep behavior:

- by default does not show the matching chunk, can be enabled with `-w`.
- doesn't work with stdin, at least one filename must be specified.
- does not have an -r (recursive) switch since that is better handled by find.
- regex flavor is Go regular expressions, as documented in
  https://github.com/google/re2/wiki/Syntax
