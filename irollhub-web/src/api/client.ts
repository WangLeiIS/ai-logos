const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

export class ApiError extends Error {
  message: string;
  code: string;
  name = 'ApiError';

  constructor(message: string, code: string) {
    super(message);
    this.message = message;
    this.code = code;
  }
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const error = await response.json().catch(() => ({
      error: 'Network error or invalid response',
      code: 'NETWORK_ERROR'
    }));
    throw new ApiError(error.error || 'Unknown error', error.code || 'UNKNOWN');
  }
  return response.json();
}

export const api = {
  search: async (q: string) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/search?q=${encodeURIComponent(q)}`);
    return handleResponse<import('../types').SearchResponse>(response);
  },

  listOrgs: async (limit = 20, offset = 0) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/orgs?limit=${limit}&offset=${offset}`);
    return handleResponse<{ data: import('../types').Organization[] }>(response);
  },

  getOrg: async (org: string) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/orgs/${org}`);
    return handleResponse<import('../types').OrgDetail>(response);
  },

  getPackages: async (org: string) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/orgs/${org}/packages`);
    return handleResponse<{ data: import('../types').Package[] }>(response);
  },

  getPackage: async (org: string, pkg: string) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/orgs/${org}/packages/${pkg}`);
    return handleResponse<import('../types').PackageDetail>(response);
  },
};
