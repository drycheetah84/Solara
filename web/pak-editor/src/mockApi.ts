// In-memory fake backend used when VITE_MOCK=1 (npm run dev:mock).
import {
  ApiError,
  type BackupInfo,
  type FileResponse,
  type Issue,
  type JobStatus,
  type PakApi,
  type PakInfo,
  type Section,
  type SettingsResponse,
  type ValidateResponse,
} from './api';

const SAMPLE_ENGINE = `[/Script/EngineSettings.GameMapsSettings]
GameDefaultMap=/Game/Maps/MainMenu.MainMenu
EditorStartupMap=/Game/Maps/MainMenu.MainMenu
GlobalDefaultGameMode=/Script/A2.A2GameMode

[/Script/Engine.Engine]
+ActiveGameNameRedirects=(OldGameName="TP_Blank",NewGameName="/Script/A2")
bSmoothFrameRate=True
SmoothedFrameRateRange=(LowerBound=(Type=Inclusive,Value=22.000000),UpperBound=(Type=Exclusive,Value=62.000000))
; intentional warning below: key without '='
bUseFixedFrameRate

[/Script/Engine.RendererSettings]
r.DefaultFeature.AutoExposure=False
r.DefaultFeature.MotionBlur=False
r.AntiAliasingMethod=2
r.Shadow.Virtual.Enable=1

[/Script/OnlineSubsystemUtils.IpNetDriver]
NetServerMaxTickRate=60
MaxClientRate=100000
MaxInternetClientRate=100000

[Core.Log]
LogNet=Warning
LogOnline=Verbose
`;

const SAMPLE_GAME = `[/Script/EngineSettings.GeneralProjectSettings]
ProjectID=6A1B0F2C4E3D4B8A9F0E1D2C3B4A5968
ProjectName=A2
ProjectVersion=1.0.30907
CompanyName=Example Studio

[/Script/UnrealEd.ProjectPackagingSettings]
BuildConfiguration=PPBC_Shipping
bShareMaterialShaderCode=True
bUseIoStore=False
+DirectoriesToAlwaysCook=(Path="/Game/Maps")

[/Script/A2.A2GameSettings]
MaxPlayers=16
RespawnDelaySeconds=5.0
bFriendlyFire=False
`;

const SAMPLE_INPUT = `[/Script/Engine.InputSettings]
bAltEnterTogglesFullscreen=True
bF11TogglesFullscreen=True
DefaultViewportMouseCaptureMode=CapturePermanently_IncludingInitialMouseDown
+ActionMappings=(ActionName="Jump",Key=SpaceBar)
+AxisMappings=(AxisName="MoveForward",Scale=1.000000,Key=W)
`;

/** Minimal ini checker mimicking the backend's validator for UI development. */
export function mockValidate(text: string): ValidateResponse {
  const issues: Issue[] = [];
  const sections: Section[] = [];
  const seen = new Map<string, number>();
  let inSection = false;
  text.split('\n').forEach((rawLine, idx) => {
    const line = idx + 1;
    const t = rawLine.trim();
    if (t === '' || t.startsWith(';') || t.startsWith('#')) return;
    if (t.startsWith('[')) {
      if (!t.endsWith(']')) {
        issues.push({ line, severity: 'error', message: 'Section header is missing closing "]"' });
        return;
      }
      const name = t.slice(1, -1).trim();
      if (!name) {
        issues.push({ line, severity: 'error', message: 'Empty section name' });
        return;
      }
      const prev = seen.get(name);
      if (prev !== undefined) {
        issues.push({ line, severity: 'warning', message: `Duplicate section [${name}] (first at line ${prev})` });
      } else {
        seen.set(name, line);
      }
      sections.push({ name, line });
      inSection = true;
      return;
    }
    if (!inSection) {
      issues.push({ line, severity: 'warning', message: 'Key outside of any section is ignored by Unreal' });
    }
    const body = t.replace(/^[+\-.!@]/, '');
    if (t.startsWith('!')) return; // "!Key" clears an array; no '=' needed
    if (!body.includes('=')) {
      issues.push({ line, severity: 'warning', message: `Line has no "=" — "${body}" will be ignored` });
      return;
    }
    if (body.startsWith('=')) {
      issues.push({ line, severity: 'error', message: 'Missing key name before "="' });
    }
    const opens = (body.match(/\(/g) ?? []).length;
    const closes = (body.match(/\)/g) ?? []).length;
    if (opens !== closes) {
      issues.push({ line, severity: 'error', message: 'Unbalanced parentheses in value' });
    }
  });
  return { issues, sections };
}

function fakeSha1(text: string): string {
  let h1 = 0x811c9dc5;
  let h2 = 0x12345678;
  for (let i = 0; i < text.length; i++) {
    const c = text.charCodeAt(i);
    h1 = Math.imul(h1 ^ c, 0x01000193) >>> 0;
    h2 = Math.imul(h2 + c, 0x5bd1e995) >>> 0;
  }
  const hex = (n: number) => n.toString(16).padStart(8, '0');
  return (hex(h1) + hex(h2) + hex(h1 ^ h2) + hex(h2 >>> 1) + hex(h1 >>> 3)).slice(0, 40);
}

const delay = (ms: number) => new Promise((r) => setTimeout(r, ms));

interface MockJob {
  started: number;
  apply: () => JobStatus['result'];
  result?: JobStatus['result'];
  fail?: string;
}

export function createMockApi(): PakApi {
  const root = 'C:\\Games\\A2\\1.0.30907\\A2\\Content';
  const pakPath = `${root}\\Paks\\A2-Windows.pak`;
  const questPath = `${root}\\Paks\\A2-Android_ASTC.pak`;
  let settings: SettingsResponse = {
    oodleDll: '',
    oodleAvailable: false,
    oodlePath: '',
    oodleSource: '',
    oodleCanCompress: false,
    oodleSupported: true,
  };
  const engineName = 'A2/Config/DefaultEngine.ini';
  const gameName = 'A2/Config/DefaultGame.ini';
  const inputName = 'A2/Config/DefaultInput.ini';
  const original: Record<string, string> = {
    [engineName]: SAMPLE_ENGINE,
    [gameName]: SAMPLE_GAME,
    [inputName]: SAMPLE_INPUT,
  };
  let files: Record<string, string> = { ...original };
  let pakSize = 231_735_296;
  const backups: BackupInfo[] = [];
  const snapshots = new Map<string, Record<string, string>>();
  const jobs = new Map<string, MockJob>();
  let jobSeq = 0;

  const paks = (): PakInfo[] => [
    {
      path: pakPath,
      name: 'A2-Windows.pak',
      size: pakSize,
      version: 8,
      entries: 48213,
      supported: true,
      configFiles: [engineName, gameName, inputName],
      compressionMethods: [],
      oodleRequired: false,
      oodleAvailable: settings.oodleAvailable,
    },
    {
      path: questPath,
      name: 'A2-Android_ASTC.pak',
      size: 74_018_153,
      version: 11,
      entries: 2832,
      supported: true,
      configFiles: [engineName, gameName, inputName],
      compressionMethods: ['Oodle'],
      oodleRequired: true,
      oodleAvailable: settings.oodleAvailable,
    },
    {
      path: `${root}\\Paks\\pakchunk1-Windows.pak`,
      name: 'pakchunk1-Windows.pak',
      size: 1_204_224_000,
      version: 11,
      entries: 9120,
      supported: false,
      configFiles: [],
      compressionMethods: ['Oodle'],
      error: 'Encrypted pak index (no AES key available)',
    },
  ];

  function startJob(apply: () => JobStatus['result']): string {
    const id = `job-${++jobSeq}`;
    jobs.set(id, { started: Date.now(), apply });
    return id;
  }

  return {
    async scan(path) {
      await delay(250);
      if (!path.trim()) throw new ApiError('Enter the path to the game Content folder', 400, 'http');
      if (/nopaks/i.test(path)) throw new ApiError(`No .pak files found under ${path}`, 404, 'http');
      return { root, paks: paks() };
    },

    async getFile(pak, name) {
      await delay(120);
      if (pak !== pakPath && pak !== questPath) throw new ApiError('Pak not found', 404, 'http');
      const compressed = pak === questPath && name !== inputName;
      if (compressed && !settings.oodleAvailable) {
        throw new ApiError(
          `entry "${name}": this entry is Oodle-compressed; the Oodle DLL is required to read it`,
          422,
          'http',
          undefined,
          'oodle_required',
        );
      }
      const text = files[name];
      if (text === undefined) throw new ApiError(`Entry ${name} not found in pak`, 404, 'http');
      const v = mockValidate(text);
      const res: FileResponse = {
        name,
        text,
        lineEnding: 'CRLF',
        bom: false,
        size: text.length + text.split('\n').length - 1,
        sha1: fakeSha1(text),
        sections: v.sections,
        issues: v.issues,
        compression: compressed ? 'Oodle' : '',
      };
      return res;
    },

    async validate(text) {
      await delay(60);
      return mockValidate(text);
    },

    async save(req) {
      await delay(150);
      if (req.pak !== pakPath && req.pak !== questPath) throw new ApiError('Pak not found', 404, 'http');
      if (req.recompress && !settings.oodleAvailable) {
        throw new ApiError('recompressing requires the Oodle DLL', 422, 'http', undefined, 'oodle_required');
      }
      const names = Object.keys(req.files);
      const changed = names.filter((n) => files[n] !== req.files[n]);
      if (changed.length === 0) throw new ApiError('No changes to save', 400, 'http');
      for (const n of names) {
        if (req.expectSha1[n] !== fakeSha1(files[n] ?? '')) {
          throw new ApiError(`${n} changed on disk since it was loaded`, 409, 'http');
        }
      }
      const errors = names.flatMap((n) => mockValidate(req.files[n]).issues.filter((i) => i.severity === 'error'));
      if (errors.length > 0) throw new ApiError('Validation failed', 422, 'http', errors);
      const id = startJob(() => {
        const stamp = new Date().toISOString().replace(/[:.]/g, '-');
        if (!backups.some((b) => b.kind === 'original')) {
          backups.push({
            path: `${pakPath}.original`,
            name: 'A2-Windows.pak.original',
            size: pakSize,
            modTime: new Date().toISOString(),
            kind: 'original',
          });
        }
        const backup: BackupInfo = {
          path: `${pakPath}.${stamp}.bak`,
          name: `A2-Windows.pak.${stamp}.bak`,
          size: pakSize,
          modTime: new Date().toISOString(),
          kind: 'timestamped',
        };
        backups.unshift(backup);
        snapshots.set(backup.path, { ...files });
        files = { ...files, ...req.files };
        pakSize += 4096;
        return {
          backup: backup.path,
          originalBackup: `${pakPath}.original`,
          newSize: pakSize,
          changed,
          recompressed: req.recompress ? changed : [],
        };
      });
      return { jobId: id };
    },

    async getJob(id) {
      await delay(40);
      const job = jobs.get(id);
      if (!job) throw new ApiError('Unknown job', 404, 'http');
      const elapsed = (Date.now() - job.started) / 2400;
      if (elapsed >= 1) {
        if (!job.result) job.result = job.apply();
        return { state: 'done', phase: 'swapping', progress: 1, result: job.result };
      }
      const phase: JobStatus['phase'] =
        elapsed < 0.4 ? 'writing' : elapsed < 0.65 ? 'verifying' : elapsed < 0.85 ? 'backing-up' : 'swapping';
      return { state: 'running', phase, progress: elapsed };
    },

    async listBackups(pak) {
      await delay(100);
      if (pak !== pakPath) return { backups: [] };
      return { backups: [...backups] };
    },

    async restore(req) {
      await delay(150);
      const b = backups.find((x) => x.path === req.backup);
      if (!b) throw new ApiError('Backup not found', 404, 'http');
      const id = startJob(() => {
        const snap = snapshots.get(b.path);
        files = b.kind === 'original' || !snap ? { ...original } : { ...snap };
        pakSize = b.size;
        return { backup: '', originalBackup: `${pakPath}.original`, newSize: b.size, changed: Object.keys(files) };
      });
      return { jobId: id };
    },

    async getSettings() {
      await delay(60);
      return { ...settings };
    },

    async saveSettings(req) {
      await delay(150);
      const p = req.oodleDll.trim();
      if (p === '') {
        settings = { ...settings, oodleDll: '', oodleAvailable: false, oodlePath: '', oodleSource: '', oodleCanCompress: false };
        return { ...settings };
      }
      if (!/(oo2core_[0-9]+_win64|oodle-data-shared)[.]dll$/i.test(p)) {
        throw new ApiError(`${p} does not export OodleLZ_Decompress (not an Oodle DLL)`, 400, 'http', undefined, 'oodle_invalid');
      }
      settings = { ...settings, oodleDll: p, oodleAvailable: true, oodlePath: p, oodleSource: 'settings', oodleCanCompress: true };
      return { ...settings };
    },
  };
}
