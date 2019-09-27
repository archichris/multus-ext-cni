#!/bin/bash


#parameters is not vaild
PARAMS_ERROR=9


################################################################################
#name:fn_rpm_ver_cmp
#input:
#	Param1:	versionA
#	Param2:	versionB
# output:
#	0 - a newer then b
#	1 - b is newer then a
#	2 - a and b are the same version
#desc: compare rpm version
################################################################################
function fn_rpm_ver_cmp()
{
  local versionA="$1"
  local versionB="$2"

  local local_verA=""
  local local_verB=""

  # The idea is to compare chunks..
  # We first split at the first non-alphanumeric (partVer / Remainder)
  #  Take the partVer and then figure out what the first character type is
  #    split the result based on the character type
  #    place the stuff after the split back in front of the Remainder
  #    compare (numerically, or alphabetically) the "result"
  #  repeat until there is nothing left to verify..

  # First check to make sure they're not equal!
  if [ "$versionA" = "$versionB" ]; then
    return 2
  fi

  while [ -n "$versionA" -a -n "$versionB" ]; do
    local_verA="`echo $versionA | sed -e 's/^[^a-zA-Z0-9]//' -e 's/\([^a-zA-Z0-9]\)/ \1/'`"
    local_verB="`echo $versionB | sed -e 's/^[^a-zA-Z0-9]//' -e 's/\([^a-zA-Z0-9]\)/ \1/'`"

    versionA="`echo "$local_verA " | cut -d ' ' -f 2`"
    local_verA="`echo "$local_verA" | cut -d ' ' -f 1`"

    versionB="`echo "$local_verB " | cut -d ' ' -f 2`"
    local_verB="`echo "$local_verB" | cut -d ' ' -f 1`"

    # isNum equal 0 if it's a number
    # isNum not equal 0 if it's not
    echo $local_verA | cut -c 1 | grep -qE "[0-9]"
    isNum=$?

    if [ $isNum -eq 0 ]; then
      local_verA="`echo $local_verA | sed 's/\([^0-9]\)/ \1/'`"
      local_verB="`echo $local_verB | sed 's/\([^0-9]\)/ \1/'`"
    else
      local_verA="`echo $local_verA | sed 's/\([^A-Za-z]\)/ \1/'`"
      local_verB="`echo $local_verB | sed 's/\([^A-Za-z]\)/ \1/'`"
    fi

    versionA="`echo "$local_verA " | cut -d ' ' -f 2`"${versionA}
    local_verA="`echo "$local_verA" | cut -d ' ' -f 1`"

    versionB="`echo "$local_verB " | cut -d ' ' -f 2`"${versionB}
    local_verB="`echo "$local_verB" | cut -d ' ' -f 1`"

    if [ -z "$local_verA" -o -z "$local_verB" ]; then
      return 1
    fi
    
    if [ $isNum -eq 0 ]; then
      if [ "$local_verA" -gt "$local_verB" ]; then return 0 ; fi
      if [ "$local_verA" -lt "$local_verB" ]; then return 1 ; fi
    else
      if [ "$local_verA" \> "$local_verB" ]; then return 0 ; fi
      if [ "$local_verA" \< "$local_verB" ]; then return 1 ; fi
    fi
  done

  # Check if anything is left over, they win
  if [ -n "$versionA" ]; then
    return 0
  fi
  if [ -n "$versionB" ]; then
    return 1
  fi

  # They were equal.. we only get here if the seperators were different!
  return 2
}

################################################################################
#name:fn_rpm_version_compare
#input:
#	Param1:	name-version-release_a
#	Param2:	name-version-release_a
# output:
#	0  - a newer then b
#	1  - b is newer then a
#	2  - a and b are the same version
#desc: compare rpm version
################################################################################
function fn_rpm_version_compare()
{
	local rpm_a=$1
	local rpm_b=$2
	local ret=2
	version_a="`echo $rpm_a | awk -F "-" '{print $(NF-1)}'`"
	release_a="`echo $rpm_a | awk -F "-" '{print $NF}'`"
	
	version_b="`echo $rpm_b | awk -F "-" '{print $(NF-1)}'`"
	release_b="`echo $rpm_b | awk -F "-" '{print $NF}'`"
	
	if [ "x$version_a" = "x" ] || [ "x$version_b" = "x" ]; then
		return $PARAMS_ERROR
	fi

	if [ "$version_a" != "$version_b" ]; then
			fn_rpm_ver_cmp $version_a $version_b
			ret=$?
			if [ $ret != 2 ]; then
					return $ret
			fi
	fi
	
	if [ "$release_a" != "$release_b" ]; then
		fn_rpm_ver_cmp $release_a $release_b
		ret=$?
	fi
	
	return $ret
}

function rpm_name_version_compare()
{
	local rpm_a=$1 
	local rpm_b=$2
	name_a="`echo $rpm_a | awk -F "-" '{print | "cut -d '-' -f -"(NF-2)}'`"
	name_b="`echo $rpm_b | awk -F "-" '{print | "cut -d '-' -f -"(NF-2)}'`"
	
	if [ "$name_a" != "$name_b" ]; then
		return $PARAMS_ERROR
	fi
	fn_rpm_version_compare $rpm_a $rpm_b
	return $?
}

