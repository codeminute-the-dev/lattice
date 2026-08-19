'use client';

import * as React from 'react';
import {cn} from '../lib/utils';

export interface NavLinkProps {
  href: string;
  children: React.ReactNode;
  /** Current pathname - if not provided, you must handle active state via isActive prop */
  pathname?: string | null;
  /** Override active state detection */
  isActive?: boolean;
  /** Called when link is clicked */
  onClick?: () => void;
  /** Custom class for active state */
  activeClassName?: string;
  /** Custom class for inactive state */
  inactiveClassName?: string;
  /** Additional classes */
  className?: string;
  /** Exact match only (default: false - will match if pathname starts with href) */
  exact?: boolean;
}

function NavLink({
  href,
  children,
  pathname,
  isActive: isActiveProp,
  onClick,
  activeClassName,
  inactiveClassName,
  className,
  exact = false,
}: NavLinkProps) {
  // Determine active state
  const isActive =
    isActiveProp ??
    (exact ? pathname === href : pathname === href || (href !== '/' && pathname?.startsWith(href)));

  const defaultActiveClassName = 'text-primary font-semibold';
  const defaultInactiveClassName = 'text-gray-700 hover:text-primary';

  return (
    <a
      href={href}
      onClick={onClick}
      className={cn(
        'rounded-md px-3 py-2 text-sm outline-none transition-colors focus:outline-none focus-visible:outline-none',
        isActive
          ? (activeClassName ?? defaultActiveClassName)
          : (inactiveClassName ?? defaultInactiveClassName),
        className
      )}
      aria-current={isActive ? 'page' : undefined}
    >
      {children}
    </a>
  );
}

export {NavLink};
