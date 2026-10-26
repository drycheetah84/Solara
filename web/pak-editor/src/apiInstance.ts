import { createHttpApi, type PakApi } from './api';
import { createMockApi } from './mockApi';
import { getSessionToken } from './session';

export const MOCK_MODE = import.meta.env.VITE_MOCK === '1';

type Listener = () => void;
const unauthorizedListeners = new Set<Listener>();

export function onUnauthorized(fn: Listener): () => void {
  unauthorizedListeners.add(fn);
  return () => unauthorizedListeners.delete(fn);
}

export function createAppApi(): PakApi {
  if (MOCK_MODE) return createMockApi();
  return createHttpApi({
    getToken: getSessionToken,
    onUnauthorized: () => unauthorizedListeners.forEach((fn) => fn()),
  });
}
