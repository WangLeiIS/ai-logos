import type { Version } from '../../types';

interface VersionListProps {
  versions: Version[];
}

export function VersionList({ versions }: VersionListProps) {
  if (versions.length === 0) {
    return (
      <div className="text-center py-8">
        <p className="text-secondary">No versions available.</p>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {versions.map((version) => (
        <div
          key={version.id}
          className="p-4 bg-white border border-border rounded-lg"
        >
          <div className="flex items-center justify-between">
            <div>
              <span className="font-semibold text-primary">{version.version}</span>
              <span className="ml-4 text-sm text-secondary">
                {new Date(version.created_at).toLocaleDateString()}
              </span>
            </div>
            <span className="text-sm text-secondary">
              {(version.file_size / 1024 / 1024).toFixed(2)} MB
            </span>
          </div>
        </div>
      ))}
    </div>
  );
}
