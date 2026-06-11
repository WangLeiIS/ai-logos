import type { Package } from '../../types';
import { PackageCard } from './PackageCard';

interface PackageListProps {
  org: string;
  packages: Package[];
}

export function PackageList({ org, packages }: PackageListProps) {
  if (packages.length === 0) {
    return (
      <div className="text-center py-12">
        <p className="text-secondary">No packages found.</p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {packages.map((pkg) => (
        <PackageCard key={pkg.id} org={org} pkg={pkg} />
      ))}
    </div>
  );
}
