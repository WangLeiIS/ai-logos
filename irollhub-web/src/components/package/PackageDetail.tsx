import type { PackageDetail as PackageDetailType } from '../../types';
import { parseTags } from '../../types';
import { VersionList } from './VersionList';

interface PackageDetailProps {
  pkg: PackageDetailType;
}

export function PackageDetail({ pkg }: PackageDetailProps) {
  const tags = parseTags(pkg.tags);

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-3xl font-bold text-primary mb-4">{pkg.name}</h1>
        <p className="text-secondary mb-4">{pkg.description || 'No description'}</p>
        <div className="flex space-x-2">
          {tags.map((tag: string) => (
            <span
              key={tag}
              className="px-3 py-1 text-sm bg-secondary text-white rounded"
            >
              {tag}
            </span>
          ))}
        </div>
      </div>

      <div>
        <h2 className="text-xl font-semibold text-primary mb-4">Versions</h2>
        <VersionList versions={pkg.versions} />
      </div>

      <div className="text-sm text-secondary">
        <p>Created: {new Date(pkg.created_at).toLocaleString()}</p>
        <p>Updated: {new Date(pkg.updated_at).toLocaleString()}</p>
        <p>Downloads: {pkg.downloads}</p>
      </div>
    </div>
  );
}
