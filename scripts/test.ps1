# This script performs tests against the dcos-diagnostics project, specifically:
#
#   * golangci-lint (https://github.com/golangci/golangci-lint)
#   * test coverage (https://blog.golang.org/cover)
#
# It outputs test and coverage reports in a way that Jenkins can understand,
# with test results in JUnit format and test coverage in Cobertura format.
# The reports are saved to build/$SUBDIR/{test-reports,coverage-reports}/*.xml 
#

$PACKAGES=""
$SOURCE_DIR=""
$SOURCE_FILES=""

function logmsg($msg)
{
    Write-Output("")
    Write-Output("*** " + $msg + " ***")
}
function fastfail($msg)
{
    if ($LASTEXITCODE -ne 0)
    {
        logmsg($msg)
        exit -1
    }
}
function fastfailpipe {
  Process  {
  fastfail("hello")
  $_
  }
}

function _golangci()
{
    & go get -u github.com/golangci/golangci-lint/cmd/golangci-lint
    fastfail("failed to get golangci-lint")
    logmsg("Running 'golangci-lint' ...")
    $text = & golangci-lint -v run
    if ($LASTEXITCODE -ne 0)
    {
        Write-Output($text)
        exit -1
    }
}

function _unittest_with_coverage {
    logmsg "Running 'go test' ..."
    $text = & go test -cover -race -v $PACKAGES
    fastfail("failed to run go test: $text") 
}

# Main.
function main {
    $SOURCE_FILES =  Get-ChildItem -File $SOURCE_DIR -Filter "*.go" -Recurse | ? { $_.FullName -inotmatch "vendor\\"  } | foreach-object  { $_.FullName }
    $PACKAGES =  & go list "./..."  | select-string -NotMatch "vendor"

    _golangci
    _unittest_with_coverage
}

main
