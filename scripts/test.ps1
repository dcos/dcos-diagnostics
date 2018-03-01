# This script performs tests against the dcos-diagnostics project, specifically:
#
#   * gofmt         (https://golang.org/cmd/gofmt)
#   * goimports     (https://godoc.org/cmd/goimports)
#   * golint        (https://github.com/golang/lint)
#   * go vet        (https://golang.org/cmd/vet)
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

function _gofmt()
{
    logmsg("Running 'gofmt' ...")
    $text = & gofmt -l $SOURCE_FILES
    if ($LASTEXITCODE -ne 0)
    {
        Write-Output($text)
        exit -1
    }
}

function _misspell 
{
    logmsg("Running 'misspell' ...")    
    & go get -u github.com/client9/misspell/cmd/misspell
    fastfail("failed to get misspell")

    $files =  Get-ChildItem -Path .  | ? { $_.BaseName -inotmatch "vendor"  } | foreach-object  { $_.BaseName }
    $text = & misspell -error $files
    if ($LASTEXITCODE -ne 0)
    {
        Write-Output($text)
        exit -1
    }
}

function _goimports() 
{
    logmsg("Running 'goimports' ...")
    & go get -u golang.org/x/tools/cmd/goimports
    fastfail("failed to get goimports")

    $text = & goimports -l $SOURCE_FILES
    if ($LASTEXITCODE -ne 0)
    {
        Write-Output($text)
        exit -1
    }
}

function _golint()
{
    logmsg("Running 'golint' ...")
    & go get -u github.com/golang/lint/
    
    $text = & golint -set_exit_status  $PACKAGES
    fastfail("failed to run golint: $text")

    if ($text.length -ne 0)
    {
        Write-Output($text)
        exit -1
    }
}

function _govet()
{
    logmsg("Running 'go vet' ...")
    $text = & go vet $PACKAGES
    fastfail("failed to run go vet $_")
    
    if ($text.length -ne 0)
    {
        Write-Output($text)
        exit -1
    }
}

function _unittest_with_coverage {
    logmsg "Running 'go test' ..."
    go test -cover -race -v $PACKAGES
}

# Main.
function main {
    $SOURCE_FILES =  Get-ChildItem -File $SOURCE_DIR -Filter "*.go" -Recurse | ? { $_.FullName -inotmatch "vendor\\"  } | foreach-object  { $_.FullName }
    $PACKAGES =  & go list "./..."  | select-string -NotMatch "vendor"

    _misspell
    _gofmt
    _goimports
    _golint
    _govet
    _unittest_with_coverage
}

main