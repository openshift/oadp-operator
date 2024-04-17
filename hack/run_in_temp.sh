#!/bin/bash

# Absolute path to the cloned oadp-operator directory
# one level up from the hack/ folder
ABS_OADP_GIT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )/.." &> /dev/null && pwd )

# Default directory where all the work will happen.
# It's inside the oadp-operator directory under the folder
# which is added to the .gitignore, so git commands of the
# parent cloned repo will not conflict 
TMP_FOLDER_NAME=".makefiletmpdir"
DEFAULT_TMP_DIR="${ABS_OADP_GIT_DIR}/${TMP_FOLDER_NAME}"

# Prefix for the folders where the code will be copied
# The full folder name will be
#    oadp_tmp-XXXX-$(git rev-parse HEAD)
OADP_TMP_DIR_PREFIX="oadp_tmp-"

# Define helper exit function which will exit on error with an message
function error_exit() {
    echo "Error: $1" >&2
    exit "${2:-1}"
}

function cleanup_and_exit() {
    echo "===== Cleaning up temporary files and exiting ====="
    local exit_val=$1
    if [ -z "${COMMAND_TEMP_DIR}" ]; then
        echo "cleanup_and_exit(): Temp dir not provided !" >&2
    else
      # Ensure dir exists and starts with prefix
      if [ -d "${COMMAND_TEMP_DIR}" ]; then
          OADP_TMP_DIR=$(basename "${COMMAND_TEMP_DIR}")
          if [[ "${OADP_TMP_DIR}" =~ "${OADP_TMP_DIR_PREFIX}"* ]]; then
              echo "Cleaning up temporary OADP files"
              rm -rf "${COMMAND_TEMP_DIR}"
              rmdir "${DEFAULT_TMP_DIR}" || exit 1
          fi
      fi
    fi

    # Propagate exit value if was provided
    [ -n "${exit_val}" ] && echo "Exit code: ${exit_val}" && exit "$exit_val"
    exit 0
}

# Cleanup on exit
trap 'cleanup_and_exit $?' TERM EXIT

# Create top level directory where subdirs will be stored
if [ ! -d "${DEFAULT_TMP_DIR}" ]; then
    mkdir -p "${DEFAULT_TMP_DIR}" || error_exit "Failed to create temporary top level directory ${DEFAULT_TMP_DIR}"
fi

# We need to get the git hash from the directory that was used to run this script
# so we get the hash of the oadp-operator source code
pushd "$ABS_OADP_GIT_DIR"
  CURRENT_GIT_HASH=$(git rev-parse HEAD)
  COMMAND_TEMP_DIR=$(TMPDIR="${DEFAULT_TMP_DIR}" mktemp -d -t "${OADP_TMP_DIR_PREFIX}XXXX-${CURRENT_GIT_HASH}") || exit 1
popd


# Copy the entire oadp directory excluding .makefiletmpdir
echo "Making a copy of the OADP folder: ${ABS_OADP_GIT_DIR}"
echo "Destination: ${COMMAND_TEMP_DIR}"
rsync -avq --exclude='${TMP_FOLDER_NAME}' "${ABS_OADP_GIT_DIR}/" "${COMMAND_TEMP_DIR}/"

# Run the commands that were passed as arguments inside the temp directory
pushd "${COMMAND_TEMP_DIR}"
  eval "$@" || exit 1
popd