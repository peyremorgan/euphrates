# Grouping Strategy Examples

These Lua scripts are starter strategies for `--grouping-dir`.

Load order is lexical by filename. Lower numbers run first. Returning `nil` or
`false` delegates to the next strategy.

## Files

- `010_common_prefix.lua`: on join, finds the existing channel with the longest
  shared prefix (minimum 4 characters after channel sigil) and reuses its
  numeric group.
- `020_delimiter_stem.lua`: on join, groups channels that share the stem before
  the first `-`, `_`, or `.` (for example `#python-dev`, `#python-help`).

## Install

Release binaries auto-install these starter scripts into
`~/.euphrates/grouping.d` on first run (without overwriting existing files).

To install manually from source:

```bash
mkdir -p ~/.euphrates/grouping.d
cp examples/grouping/*.lua ~/.euphrates/grouping.d/
```

Then start Euphrates normally, or point at another strategy directory with:

```bash
./euphrates --grouping-dir /path/to/grouping.d ...
```