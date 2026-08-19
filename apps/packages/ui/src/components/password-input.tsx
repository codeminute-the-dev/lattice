'use client';

import * as React from 'react';
import {Input} from './input';
import {Button} from './button';
import {Eye, EyeOff} from 'lucide-react';
import {cn} from '../lib/utils';

interface PasswordInputProps extends React.ComponentProps<'input'> {
  showPassword?: boolean;
  onShowPasswordChange?: (show: boolean) => void;
}

export function PasswordInput({
  className,
  showPassword = false,
  onShowPasswordChange,
  ...props
}: PasswordInputProps) {
  const [showPasswordInternal, setShowPasswordInternal] = React.useState(false);

  const isControlled = onShowPasswordChange !== undefined;
  const show = isControlled ? showPassword : showPasswordInternal;
  const setShow = isControlled ? onShowPasswordChange : setShowPasswordInternal;

  return (
    <div className="relative">
      <Input type={show ? 'text' : 'password'} className={cn('pr-10', className)} {...props} />
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="absolute right-2 top-1/2 -translate-y-1/2 hover:bg-transparent"
        onClick={() => setShow(!show)}
      >
        {show ? (
          <EyeOff className="text-muted-foreground h-4 w-4" />
        ) : (
          <Eye className="text-muted-foreground h-4 w-4" />
        )}
        <span className="sr-only">{show ? 'Hide password' : 'Show password'}</span>
      </Button>
    </div>
  );
}
