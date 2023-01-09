$Excel=New-Object -ComObject Excel.Application
$Excel.Visible = $true

for ($i=0; $i -lt $($args.Length - 1); $i++)
{
    $inputWb = $Excel.Workbooks.Open($($args[$i]))
}

$outputWb=$Excel.Workbooks.Open($($args[$($args.Length - 1)]))
$outputWb.RefreshAll()
While (($outputWb.Sheets | ForEach-Object {$_.QueryTables | ForEach-Object {if($_.QueryTable.Refreshing){$true}}}))
{
    Start-Sleep -Seconds 1
}
$outputWb.Save()
$outputWb.Close()
$Excel.Quit()