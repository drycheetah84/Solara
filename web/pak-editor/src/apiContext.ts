import { createContext, useContext } from 'react';
import type { PakApi } from './api';

export const ApiContext = createContext<PakApi | null>(null);

export function useApi(): PakApi {
  const api = useContext(ApiContext);
  if (!api) throw new Error('ApiContext not provided');
  return api;
}
