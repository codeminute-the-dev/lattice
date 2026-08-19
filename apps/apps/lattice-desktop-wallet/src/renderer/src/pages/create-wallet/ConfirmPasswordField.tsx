import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Button} from '@/components/ui/button';
import {Eye, EyeOff} from 'lucide-react';

type ConfirmPasswordFieldProps = {
  onChange: (v: string) => void;
  showConfirmPassword: boolean;
  setShowConfirmPassword: (v: boolean) => void;
  disabled?: boolean;
};

export default function ConfirmPasswordField({
  onChange,
  showConfirmPassword,
  setShowConfirmPassword,
  disabled,
}: ConfirmPasswordFieldProps) {
  return (
    <div className="space-y-2">
      <Label htmlFor="confirmPassword" className="text-gray-900">
        Confirm Password
      </Label>
      <div className="relative">
        <Input
          type={showConfirmPassword ? 'text' : 'password'}
          onChange={e => onChange(e.target.value)}
          placeholder="Confirm your password"
          className="focus:border-brand-green focus:ring-brand-green/20 border-gray-300 bg-white pr-10 text-gray-900 shadow-sm focus:ring-2"
          disabled={disabled}
        />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="absolute right-1 top-1 h-8 w-8 text-gray-500 hover:bg-gray-100 hover:text-gray-700"
          onClick={() => setShowConfirmPassword(!showConfirmPassword)}
        >
          {showConfirmPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </Button>
      </div>
    </div>
  );
}
