import { describe, expect, it, vi } from 'vitest';
import { ApiError, createHttpApi } from '../api';

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

describe('api client', () => {
  it('sends the X-Session-Token header on every request', async () => {
    const fetchImpl = vi.fn(async () => jsonResponse({ root: 'C:/x', paks: [] }));
    const api = createHttpApi({ getToken: () => 'secret-token', fetchImpl });
    await api.scan('C:/Game/Content');
    await api.getFile('C:/p.pak', 'A2/Config/DefaultEngine.ini');

    expect(fetchImpl).toHaveBeenCalledTimes(2);
    const [url, init] = fetchImpl.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/scan');
    expect(init.method).toBe('POST');
    expect((init.headers as Record<string, string>)['X-Session-Token']).toBe('secret-token');
    expect(JSON.parse(init.body as string)).toEqual({ path: 'C:/Game/Content' });

    const [url2, init2] = fetchImpl.mock.calls[1] as unknown as [string, RequestInit];
    expect(url2).toBe('/api/file?pak=C%3A%2Fp.pak&name=A2%2FConfig%2FDefaultEngine.ini');
    expect((init2.headers as Record<string, string>)['X-Session-Token']).toBe('secret-token');
  });

  it('maps error bodies and reports 401 to the unauthorized handler', async () => {
    const onUnauthorized = vi.fn();
    const fetchImpl = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ error: 'bad token' }, 401))
      .mockResolvedValueOnce(
        jsonResponse({ error: 'validation failed', issues: [{ line: 3, severity: 'error', message: 'x' }] }, 422),
      );
    const api = createHttpApi({ getToken: () => 't', fetchImpl, onUnauthorized });

    await expect(api.validate('a')).rejects.toMatchObject({ status: 401, message: 'bad token' });
    expect(onUnauthorized).toHaveBeenCalledTimes(1);

    const err = await api.save({ pak: 'p', files: { a: 'b' }, expectSha1: { a: 'c' } }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(422);
    expect((err as ApiError).issues).toHaveLength(1);
  });

  it('turns fetch failures into network errors', async () => {
    const fetchImpl = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'));
    const api = createHttpApi({ getToken: () => 't', fetchImpl });
    await expect(api.listBackups('p')).rejects.toMatchObject({ kind: 'network', status: 0 });
  });
});
