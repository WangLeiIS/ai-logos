export interface Organization {
  id: number;
  name: string;
  avatar_url: string;
  created_at: string;
}

export interface Package {
  id: number;
  name: string;
  description: string;
  tags: string;  // JSON string array
  downloads: number;
  created_at: string;
  updated_at: string;
}

export interface Version {
  id: number;
  version: string;
  object_key: string;
  file_size: number;
  checksum: string;
  created_at: string;
}

export interface PackageDetail extends Package {
  versions: Version[];
}

export interface OrgDetail extends Organization {
  packages: Package[];
}

export interface SearchResponse {
  data: Array<{
    org: Organization;
    package: Package;
  }>;
}

export interface ApiError {
  error: string;
  code: string;
}
