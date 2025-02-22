#!/usr/bin/env bash
#
#   Copyright The OpenTelemetry Authors.
#
#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at
#
#       http://www.apache.org/licenses/LICENSE-2.0
#
#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.
#
# This script checks for issues that have been inactive for a certain number
# of days. Any inactive issues have codeowners pinged for labels corresponding
# to a component and are marked as stale. The stale bot will then handle
# the rest of the lifecycle, including removing the stale label and closing
# the issue.
#
# This script is necessary instead of just using the stale action because
# the stale action does not support pinging code owners, and pinging
# code owners after marking an issue as stale will cause the issue to
# have the stale label removed according to all documented behavior
# of the stale action.

set -euo pipefail

if [[ -z ${DAYS_BEFORE_STALE} || -z ${DAYS_BEFORE_CLOSE} || -z ${STALE_LABEL} || -z ${EXEMPT_LABEL} ]]; then
    echo "At least one of DAYS_BEFORE_STALE, DAYS_BEFORE_CLOSE, STALE_LABEL, or EXEMPT_LABEL has not been set, please ensure each is set."
    exit 0
fi

STALE_MESSAGE="This issue has been inactive for ${DAYS_BEFORE_STALE} days. It will be closed in ${DAYS_BEFORE_CLOSE} days if there is no activity."

# Check for the least recently-updated issues that aren't currently stale.
# If no issues in this list are stale, the repo has no stale issues.
ISSUES=(`gh issue list --search "is:issue is:open -label:${STALE_LABEL} -label:\"${EXEMPT_LABEL}\" sort:updated-asc" --json number --jq '.[].number'`)

for ISSUE in "${ISSUES[@]}"; do
    OWNERS=''

    UPDATED_AT=`gh issue view ${ISSUE} --json updatedAt --jq '.updatedAt'`
    UPDATED_UNIX=`date +%s --date="${UPDATED_AT}"`
    NOW=`date +%s`
    DIFF_DAYS=$(($((${NOW}-${UPDATED_UNIX}))/(3600*24)))    

    if [[ ${DIFF_DAYS} < ${DAYS_BEFORE_STALE} ]]; then
        # echo "Issue #${ISSUE} is not stale. Issues are sorted by updated date in ascending order, so all remaining issues must not be stale. Exiting."
        exit 0
    fi

    LABELS=(`gh issue view ${ISSUE} --json labels --jq '.labels.[].name'`)

    for LABEL in "${LABELS[@]}"; do
        if ! [[ ${LABEL} =~ ^cmd/ || ${LABEL} =~ ^confmap/ || ${LABEL} =~ ^exporter/ || ${LABEL} =~ ^extension/ || ${LABEL} =~ ^internal/ || ${LABEL} =~ ^pkg/ || ${LABEL} =~ ^processor/ || ${LABEL} =~ ^receiver/ ]]; then
            continue
        fi

        COMPONENT=${LABEL}
        result=`grep -c ${LABEL} .github/CODEOWNERS`

        # there may be more than 1 component matching a label
        # if so, try to narrow things down by appending the component
        # type to the label
        if [[ $result != 1 ]]; then
            COMPONENT_TYPE=`echo ${COMPONENT} | cut -f 1 -d '/'`
            COMPONENT="${COMPONENT}${COMPONENT_TYPE}"
        fi

        OWNERS+="- ${COMPONENT}: `grep -m 1 ${COMPONENT} .github/CODEOWNERS | sed 's/   */ /g' | cut -f3- -d ' '`\n"
    done

    if [[ -z "${OWNERS}" ]]; then
        echo "No code owners found. Marking issue as stale without pinging code owners."

        gh issue comment ${ISSUE} -b "${STALE_MESSAGE}"
    else
        echo "Pinging code owners for issue #${ISSUE}."

        # The GitHub CLI only offers multiline strings through file input.
        printf "${STALE_MESSAGE} Pinging code owners:\n${OWNERS}\nSee [Adding Labels via Comments](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/CONTRIBUTING.md#adding-labels-via-comments) if you do not have permissions to add labels yourself." \
          | gh issue comment ${ISSUE} -F -
    fi

    # We want to add a label after making a comment for two reasons:
    # 1. If there is some error making a comment, a stale label should not be applied.
    #    We want code owners to be pinged before closing an issue as stale.
    # 2. The stale bot (as of v6) uses the timestamp for when the stale label was
    #    applied to determine when an issue was marked stale. We want to ensure that
    #    was the last activity on the issue, or the stale bot will remove the stale
    #    label if our comment to ping code owners comes too long after the stale
    #    label is applied.
    gh issue edit ${ISSUE} --add-label "${STALE_LABEL}"
done

