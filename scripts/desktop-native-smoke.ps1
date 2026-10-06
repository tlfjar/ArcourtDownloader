param(
    [Parameter(Mandatory)][ValidateSet('choose', 'open', 'close', 'deny', 'restore', 'cleanup', 'inspect', 'startup', 'capture')][string]$Action,
    [int]$AppProcessId,
    [string]$Folder
)
$ErrorActionPreference = 'Stop'
if ($Action -eq 'startup') {
    $deadline = [DateTime]::UtcNow.AddSeconds(20)
    do {
        $app = Get-Process -Id $AppProcessId
        if ($app.MainWindowTitle -eq 'Arcourt Downloader' -and $app.MainWindowHandle -ne 0) {
            Write-Output 'Normal executable started with a visible native window.'
            exit 0
        }
        Start-Sleep -Milliseconds 200
    } while ([DateTime]::UtcNow -lt $deadline)
    throw 'Normal executable did not show its window.'
}
if ($Action -eq 'capture') {
    Add-Type -AssemblyName System.Drawing
    Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class FixtureCapture {
    [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out RECT rect);
    [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr hWnd, IntPtr hDC, uint flags);
}
'@
    $handle = (Get-Process -Id $AppProcessId).MainWindowHandle
    $rect = New-Object FixtureCapture+RECT
    if (-not [FixtureCapture]::GetWindowRect($handle, [ref]$rect)) { throw 'App window bounds unavailable.' }
    $bitmap = New-Object System.Drawing.Bitmap(($rect.Right-$rect.Left), ($rect.Bottom-$rect.Top))
    $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
    $dc = $graphics.GetHdc()
    try { if (-not [FixtureCapture]::PrintWindow($handle, $dc, 2)) { throw 'App window capture failed.' } }
    finally { $graphics.ReleaseHdc($dc); $graphics.Dispose() }
    try { $bitmap.Save($Folder, [System.Drawing.Imaging.ImageFormat]::Png) } finally { $bitmap.Dispose() }
    exit 0
}
if ($Action -eq 'close') {
    if (-not (Get-Process -Id $AppProcessId).CloseMainWindow()) { throw 'Window close could not be requested.' }
    exit 0
}
if ($Action -eq 'deny' -or $Action -eq 'restore') {
    $acl = [System.IO.Directory]::GetAccessControl($Folder)
    $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
    $rule = [System.Security.AccessControl.FileSystemAccessRule]::new($identity, 'Write', 'ContainerInherit,ObjectInherit', 'None', 'Deny')
    if ($Action -eq 'deny') { $acl.AddAccessRule($rule) } else { $acl.RemoveAccessRuleSpecific($rule) }
    [System.IO.Directory]::SetAccessControl($Folder, $acl)
    exit 0
}
if ($Action -eq 'cleanup') {
    $owned = @(Get-CimInstance Win32_Process | Where-Object {
        $_.Name -in @('msedge.exe','chrome.exe') -and $_.CommandLine -and $_.CommandLine.Contains($Folder)
    })
    if ($owned.Count) { throw 'Application-owned automation browser is still running.' }
    if (@(Get-ChildItem -LiteralPath $Folder -Directory -Filter 'arcourt-browser-*').Count) { throw 'Application-owned browser profile remains.' }
    exit 0
}
if ($Action -eq 'open') {
    $shell = New-Object -ComObject Shell.Application
    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    do {
        foreach ($window in $shell.Windows()) {
            if ($window.LocationURL -and ([Uri]$window.LocationURL).LocalPath.TrimEnd('\') -eq $Folder.TrimEnd('\')) {
                $window.Quit() # This exact test-only directory was opened by the app.
                Write-Output 'Explorer opened the exact output folder.'
                exit 0
            }
        }
        Start-Sleep -Milliseconds 200
    } while ([DateTime]::UtcNow -lt $deadline)
    throw 'Explorer did not expose the expected folder.'
}
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
$root = [System.Windows.Automation.AutomationElement]::RootElement
if ($Action -eq 'inspect') {
    $condition = [System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::ProcessIdProperty, $AppProcessId)
    $elements = $root.FindAll([System.Windows.Automation.TreeScope]::Descendants, $condition)
    $elements | ForEach-Object { '{0} | {1}' -f $_.Current.ControlType.ProgrammaticName,$_.Current.Name }
    exit 0
}
$condition = [System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty, 'Choose PDF output folder')
$deadline = [DateTime]::UtcNow.AddSeconds(15)
do {
    $dialog = $root.FindFirst([System.Windows.Automation.TreeScope]::Descendants, $condition)
    if ($null -ne $dialog -and $dialog.Current.ProcessId -eq $AppProcessId) { break }
    Start-Sleep -Milliseconds 200
} while ([DateTime]::UtcNow -lt $deadline)
if ($null -eq $dialog) { throw 'Native folder picker did not appear.' }
$elements = $dialog.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
$edit = $null
$button = $null
foreach ($element in $elements) {
    if ($element.Current.AutomationId -eq '1152') { $edit = $element }
    if ($element.Current.AutomationId -eq '1') { $button = $element }
}
if ($null -eq $edit -or $null -eq $button) {
    $elements | ForEach-Object { '{0} | {1} | {2}' -f $_.Current.ControlType.ProgrammaticName,$_.Current.AutomationId,$_.Current.Name }
    throw 'Native picker controls were not found (English Windows smoke harness).'
}
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class FixtureDialog {
    [DllImport("user32.dll", CharSet=CharSet.Unicode)]
    public static extern IntPtr SendMessage(IntPtr hWnd, uint msg, IntPtr wParam, string lParam);
    [DllImport("user32.dll")]
    public static extern bool PostMessage(IntPtr hWnd, uint msg, IntPtr wParam, IntPtr lParam);
}
'@
# The standard Windows picker sometimes exposes these Win32 controls as Panes
# instead of Edit/Button patterns. Address their native handles, not screen coordinates.
[FixtureDialog]::SendMessage([IntPtr]$edit.Current.NativeWindowHandle, 0x000C, [IntPtr]::Zero, $Folder) | Out-Null
[FixtureDialog]::PostMessage([IntPtr]$button.Current.NativeWindowHandle, 0x00F5, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
Write-Output 'Native folder picker accepted the test path with spaces.'
