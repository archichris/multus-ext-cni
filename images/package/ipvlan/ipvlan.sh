#!/bin/bash

######## global variable ########
EULER_OS="EulerOS \
EulerOS23 \
EulerOS25 \
EulerOS-Arm64"

CENT_OS="CENTOS74 \
CENTOS75 \
CENTOS76"

OTHER_OS="RHEL71"

RHEL_OS="${EULER_OS} ${CENT_OS} ${OTHER_OS}"
supported_os="${EULER_OS} ${CENT_OS} ${OTHER_OS}"
depmod_os="EulerOS EulerOS23 ${CENT_OS}"

absolute_path="$(cd "$(dirname "$0")"; pwd)"
ipvlan_rpm="ipvlan"

######## functions ########
source ${absolute_path}/rpm_version_compare.sh

echo -e `date "+%Y-%m-%d %H:%M:%S.%N"` "Start the script:\033[32m $absolute_path/$0\033[0m, User: \033[32m`whoami`\033[0m"
echo `date "+%Y-%m-%d %H:%M:%S.%N"` "User infomation: `who am i`"

function fn_log()
{
    local loglevel="$1"
    shift 1
    local logmsg="$@"
    local bash_name="ipvlan.sh"
    echo "$(date "+%Y-%m-%d %H:%M:%S.%N") ${bash_name}:[ ${loglevel} ] ${logmsg}"
}

function judge_os()
{
    grep -i -w "EulerOS" /etc/os-release > /dev/null 2>&1
    if [[ $? -eq 0 ]] || [[ -f /etc/euleros-latest ]] || [[ -f /etc/EulerLinux.conf ]];then
        uname -r|grep "aarch64" >/dev/null 2>&1
        if [[ $? -eq 0 ]];then
            echo "EulerOS-Arm64"
            return 0
        fi

        if [[ -f /etc/euleros-release ]];then
            version=`cat /etc/euleros-release`
            if [[ "$version" = "EulerOS release 2.0 (SP2)" ]];then
                echo "EulerOS"
                return 0

            elif [[ "$version" = "EulerOS release 2.0 (SP3)" ]];then
                echo "EulerOS23"
                return 0

            elif [[ "$version" = "EulerOS release 2.0 (SP5)" ]];then
                echo "EulerOS25"
                return 0
            fi
        fi

        return 0
    fi

    if [[ -f /etc/redhat-release ]];then
        version=$(grep 'Server release' /etc/redhat-release | awk '{print $7}')
        if [[ "$version"x == x ]]; then
            version=$(grep 'CentOS Linux release' /etc/redhat-release | awk '{print $4}')
            if [[ "$version"x = 7.4*x ]];then
                echo "CENTOS74"
                return 0
            elif [[ "$version"x = 7.5*x ]];then
                echo "CENTOS75"
                return 0
            elif [[ "$version"x = 7.6*x ]];then
                echo "CENTOS76"
                return 0
            fi
        else
            if [[ "$version"x = "7.1"x ]];then
                echo "RHEL71"
                return 0
            fi
        fi
    fi

    fn_log ERROR "OS is not supported!"
    return 1
}

function remove_kmod()
{
    lsmod | grep ipvlan > /dev/null 2>&1 && rmmod -f ipvlan
    return 0
}

function check_installed_rpm_files_valid()
{
    local input=$1
    local file=""
    local flag=0
    local file_list=""

    file_list="$(rpm -ql $input)"
    for file in $file_list
    do
        test -e "$file" > /dev/null 2>&1
        if [ $? -ne 0 ];then
            fn_log ERROR "$input: missing $file"
            flag=1
            break
        fi
    done
    return $flag
}

function depmod_in_centos()
{
    # install or uninstall
    action=$1
    ipvlan_mod_path=$(rpm -qal ipvlan | grep ko)
    rpm_version=$(rpm -qal ipvlan|grep -w ipvlan.ko|head -1|awk -F "/" '{print $4}')
    sys_version=$(uname -r)

    if [[ "${rpm_version}" != "${sys_version}" ]];then
        mkdir -p /lib/modules/"${sys_version}"/extra/ipvlan -m 750
        fn_log INFO "Depmod in ${OS} for ${action} IPVlan"
        for path in ${ipvlan_mod_path[@]};do
            name=$(echo "$path" | awk -F "/" '{print $NF}')
            rm -f /lib/modules/"$(uname -r)"/extra/ipvlan/"${name}"
            if [ "$action" = "install" ];then
                ln -s "${path}" /lib/modules/"$(uname -r)"/extra/ipvlan/"${name}"
            fi
        done

        depmod "$(uname -r)"
    fi
    depmod "$(uname -r)"
}

function check_is_installed()
{
    local ipvlan_file_list=0
    local rpm_file_list=""
    local file=""
    local installed=0
    local valid=1

    rpm -q "$ipvlan_rpm" > /dev/null 2>&1 && ipvlan_file_list=1
    if [ "$ipvlan_file_list" -eq 1 ];then
        rpm_file_list="$ipvlan_rpm"
        installed=1
    fi

    for file in $rpm_file_list;do
        check_installed_rpm_files_valid "$file" > /dev/null 2>&1
        if [ $? -ne 0 ];then
            valid=0
            break
        fi
    done

    if [ "$installed" -eq 1 ] && [ "$valid" -eq 1 ];then
        # All packages are install correctly and no file is missing
        return 0
    else
        return 1
    fi
}

function check_nf_conntrack_ipv4()
{
    lsmod | grep 'nf_conntrack_ipv4 '
    if [ $? -eq 0 ];then
       return 0
    fi

     fn_log INFO "nf_conntrack_ipv4 not exist, need load...."

     modprobe nf_conntrack_ipv4
     if [ $? -ne 0 ];then
        fn_log "WARNNING" "Failed to modprobe nf_conntrack_ipv4"
        return 1
     fi
     fn_log INFO "nf_conntrack_ipv4 load success"
     return 0
}

function is_uninstalled()
{
    local ipvlan=0

    rpm -q "$ipvlan_rpm" > /dev/null 2>&1 && ipvlan=1
    if [ "$ipvlan" -eq 0 ];then
        echo "true"
        return 0
    else
        echo "false"
        return 1
    fi
}

function install_ipvlan()
{
    fn_log INFO "Start to install ipvlan..."
    check_nf_conntrack_ipv4

    check_is_installed
    if [ $? -eq 0 ];then
        fn_log INFO "ipvlan is already installed."
        return 0
    fi

    rpm_list="ipvlan"

    fn_log INFO "Installing RPM packages..."
    for file in $rpm_list;do
        rpm -q ${file} > /dev/null 2>&1
        if [ $? -eq 0 ];then
            fn_log INFO "Checking validation for package ${file}..."
            check_installed_rpm_files_valid "$file"
            if [ $? -ne 0 ];then
                fn_log "ERROR" "Some files for $file are missing. Please remove $file and try again."
                return 1
            fi
            fn_log INFO "${file} is valid, skipping this package..."
            continue
        fi
        fn_log INFO "Installing ${absolute_path}/${OS}/${file}-*.rpm"
        rpm -ivh "${absolute_path}"/"$OS"/"$file-*.rpm"
        if [ $? -ne 0 ];then
            fn_log "ERROR" "Failed to install $file-*.rpm"
            return 1
        fi
    done

    fn_log INFO "Removing kmod..."
    remove_kmod
    if [ $? -ne 0 ];then
        return 1
    fi

    for name in ${depmod_os}; do
        if [[ "${OS}" = "${name}" ]]; then
            fn_log INFO "Depmod modules in ${OS}..."
            depmod_in_centos "install"
            break
        fi
    done

    fn_log INFO "Chkconfig ipvlan on..."
    chkconfig ipvlan on

    start_service_result=$(systemctl start ipvlan 2>&1)
    if [ $? -ne 0 ];then
        fn_log "ERROR" "Failed to start ipvlan service, $start_service_result"
        return 1
    fi

    check_is_installed
    if [ $? -eq 0 ];then
        fn_log INFO "IPVlan installation is finished."
    else
        fn_log ERROR "Failed to install IPVlan."
        return 1
    fi

    return 0
}

function uninstall_ipvlan()
{
    fn_log INFO "Start to uninstall IPVlan..."
    ret=$(is_uninstalled)
    if [ "$ret" = "true" ];then
        fn_log INFO "ipvlan is already uninstalled."
        return 0
    fi

    fn_log INFO "Stop ipvlan service..."
    stop_service_result=$(systemctl stop ipvlan)
    if [ $? -ne 0 ];then
        fn_log "Warning" "Failed to stop ipvlan service, $stop_service_result"
    fi

    fn_log INFO "Removing kmod..."
    remove_kmod
    if [ $? -ne 0 ];then
        return 1
    fi

    for name in ${depmod_os}; do
        if [[ "${OS}" = "${name}" ]]; then
            fn_log INFO "Depmod modules in ${OS}..."
            depmod_in_centos "uninstall"
            break
        fi
    done

    rpm_list="$ipvlan_rpm"

    fn_log INFO "Uninstalling RPM packages..."
    for file in $rpm_list;do
        rpm -q ${file} > /dev/null 2>&1
        if [ $? -ne 0 ];then
            continue
        fi

        fn_log INFO "rpm -e $file ..."
        rpm -e "$file" > /dev/null 2>&1
        if [ $? -ne 0 ];then
            fn_log ERROR "Failed to uninstall $file."
            return 1
        fi
    done

    ret=$(is_uninstalled)
    if [ "$ret" = "true" ];then
        fn_log INFO "Ipvlan uninstallation is finished."
    else
        fn_log ERROR "Failed to uninstall IPVlan."
        return 1
    fi

    return 0
}

function update_package()
{
    local package_to_update=$1
    local package_in_sys=""
    local rpm_name=""
    local rpm_package="${RPM_DIR}/${package_to_update}.rpm"
    local result=0

    rpm_name=$(echo "${package_to_update}" | awk -F "-" '{print | "cut -d '-' -f -"(NF-2)}')

    # No package installed in current node, install it directly.
    package_in_sys=$(rpm -q "${rpm_name}")
    if [ $? -ne 0 ];then
        fn_log INFO "${rpm_name} is not installed, install it."

        rpm -ivh "${rpm_package}"
        if [ $? -ne 0 ];then
            fn_log ERROR "Failed to install ${rpm_name}"
            return 1
        fi
        return 0
    fi
    if [ "${OS}" = "SLES11" ];then
        package_in_sys="${package_in_sys}.x86_64"
    fi

    # 0 means version is bigger than current version
    # 1 means version is lower than current version
    # 2 means version is same with current version
    # other means error
    rpm_name_version_compare ${package_to_update} ${package_in_sys}
    result=$?
    if [ ${result} -eq 2 ];then
        fn_log WARN "Updating version is same with current version, skip updating ${package_to_update}"
        return 0
    elif [ ${result} -eq 1 ];then
        fn_log WARN "Updating version (${package_to_update}) is older than current version (${package_in_sys}), skip updating"
        return 0
    elif [ ${result} -ne 0 ];then
        fn_log ERROR "Failed to compare two package (result ${result}): ${package_to_update} ${package_in_sys}"
        return 1
    fi

    # Version is newer than current
    fn_log INFO "Stop service to update ipvlan package..."
    stop_service_result=$(systemctl stop ipvlan)
    if [ $? -ne 0 ];then
        fn_log "Warning" "Failed to stop ipvlan service, $stop_service_result"
    fi
    rpm -Uvh "${rpm_package}"
    if [ $? -ne 0 ];then
        fn_log ERROR "Failed to update ${rpm_package}"
        return 1
    fi

    fn_log INFO "Chkconfig ipvlan on and start ipvlan service..."
    chkconfig ipvlan on
    start_service_result=$(systemctl start ipvlan 2>&1)
    if [ $? -ne 0 ];then
        fn_log "ERROR" "Failed to start ipvlan service, $start_service_result"
        return 1
    fi

    return 0
}

function update_ipvlan()
{
    fn_log INFO "Start to update ipvlan..."
    local packages_to_update=$(basename $(find ${RPM_DIR} -name "${ipvlan_rpm}-*.rpm") .rpm)
    fn_log INFO "Packages to update: ${packages_to_update}"

    for package in ${packages_to_update}
    do
        update_package ${package}
        if [ $? != 0 ];then
            fn_log ERROR "Failed to update ${package}"
            return 1
        fi
    done

    for name in ${depmod_os}; do
        if [[ "${OS}" = "${name}" ]]; then
            fn_log INFO "Depmod modules in ${OS}..."
            depmod_in_centos "install"
            break
        fi
    done

    fn_log INFO "Removing kmod..."
    remove_kmod
    if [ $? -ne 0 ];then
        return 1
    fi

    fn_log INFO "Finish updating ipvlan."
}

function check_os_type()
{
    local osname=$1

    if [[ "$osname" = "EulerOS-Arm64" ]];then
        echo "ARM64"
        return 0
    fi

    for name in ${RHEL_OS}; do
        if [[ "${name}" = "${osname}" ]]; then
            echo "RHEL"
            return 0
        fi
    done


    return 0
}

function usage()
{
    echo "Usage: sh $0 [install|uninstall|update]"
}

######### Entry point ##########
fn_log INFO "Checking OS name..."
OS="$(judge_os)"
if [ $? -ne 0 ];then
    echo "OS is not supported. Supported OS: ${supported_os}"
    exit 1
fi

fn_log INFO "OS name is ${OS}..."
OS_TYPE="$(check_os_type "${OS}")"
fn_log INFO "OS type is ${OS_TYPE}..."
RPM_DIR="${absolute_path}/${OS}"

case "$1" in
    install)
        install_ipvlan "$OS_TYPE"
        exit $?
        ;;
    uninstall)
        uninstall_ipvlan "$OS_TYPE"
        exit $?
        ;;
    update)
        update_ipvlan "$OS_TYPE"
        exit $?
        ;;
    *)
        # error
        usage
        exit 1
        ;;
esac
