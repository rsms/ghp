# has_newer returns 0 (true) if dir contains any file newer than ref_file
#
# has_newer ref_file, dir, name_pattern, exclude_dir -> int
# has_newer ref_file, dir, name_pattern -> int
has_newer() {
  ref_file=$1
  dir=$2
  name_pattern=$3
  if [[ ! -f "$ref_file" ]]; then
    return 0
  fi
  if [[ -z $4 ]]; then
    for f in $(\
      find "$dir" \
        -type f -name "$name_pattern" -newer "$ref_file" -print -quit \
    ); do
      return 0
    done
  else
    exclude_dir=$4
    for f in $(\
      find "$dir" \
        -type d -name "$exclude_dir" -prune \
        -o \
        -type f -name "$name_pattern" -newer "$ref_file" -print -quit \
    ); do
      return 0
    done
  fi
  return 1
}

# like has_newer but uses regexp for name_pattern and exclude_pattern
#
# has_newer ref_file, dir, name_pattern, exclude_pattern -> int
# has_newer ref_file, dir, name_pattern -> int
has_newer_re() {
  ref_file=$1
  dir=$2
  name_pattern=$3
  if [[ ! -f "$ref_file" ]]; then
    return 0
  fi
  if [[ -z $4 ]]; then
    for f in $(\
      find -E "$dir" \
        -type f -regex "$name_pattern" -newer "$ref_file" -print -quit \
    ); do
      return 0
    done
  else
    exclude_pattern=$4
    for f in $(\
      find -E "$dir" \
        -type f -regex "$name_pattern" -not -regex "$exclude_pattern" \
        -newer "$ref_file" -print -quit \
    ); do
      return 0
    done
  fi
  return 1
}
