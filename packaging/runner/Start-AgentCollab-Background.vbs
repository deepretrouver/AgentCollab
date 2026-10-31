Option Explicit

Dim shell, fso, launchScript
Set shell = CreateObject("WScript.Shell")
Set fso = CreateObject("Scripting.FileSystemObject")
launchScript = fso.BuildPath(fso.GetParentFolderName(WScript.ScriptFullName), "Run-AgentCollab-Background.cmd")

If fso.FileExists(launchScript) Then
  shell.Run Chr(34) & launchScript & Chr(34), 0, False
End If
