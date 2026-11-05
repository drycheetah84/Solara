@{
    # Halcyon game box (Azure Windows VM) -- A2 1.0.50451, backend = Solara on the Azure hub (10.10.1.4).
    # Read by HalcyonAgent.ps1 for every spin-up.
    GameBuild     = '50451'
    GameExe       = 'C:\Halcyon\game\A2\Binaries\Win64\A2-Win64-Shipping.exe'
    Configuration = 'Release'
    DllPath       = 'C:\Halcyon\HalcyonA2.dll'

    InitWaitSeconds = 60

    # REQUIRED on every production game server: the SpecEditor flags -SpecEdit and -SpecEditSandbox (the in-game
    # level editor and its synced sandbox placement). Never drop them from a deploy. Older DLLs have the editor OFF
    # unless these are passed; without them the editor client's placements, edits and exports are silently ignored.
    # The agent reads the box's OWN copy, C:\Halcyon\server.config.psd1: a deploy must copy this file there too
    # (docs/agent-notes/solara-cloudflare-tunnel.md), and only servers started afterwards get the new flags.
    Args = @(
        '-nullrhi'
        '-nosound'
        '-log'
        '-SpecEdit'
        '-SpecEditLocalTest'
        '-SpecEditSandbox'
        '-NoSpecBlock'
    )

    # Backend on the private network. 127.0.0.1:78 / :90 are also port-proxied to 10.10.1.4 by
    # bootstrap.ps1, because the DLL's DashboardApiUrl default is http://127.0.0.1:78.
    DashboardApiUrl = 'http://10.10.1.4:78'
    DeploymentId    = ''

    # Blank = the DLL asks checkip.amazonaws.com for this box's PUBLIC ip (what clients connect to).
    RegisterIp = ''

    ExtraArgs = @(
        '-BackendHost=10.10.1.4'
        '-MothershipHost=10.10.1.4'
        '-ServerName=Halcyon'
    )
}
