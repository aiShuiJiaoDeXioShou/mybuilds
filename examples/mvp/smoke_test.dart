import 'package:flutter_test/flutter_test.dart';
import 'package:mvp_flutter/main.dart';

void main() {
  testWidgets('构建渠道显示在页面中', (tester) async {
    await tester.pumpWidget(const MainApp());
    const channel = String.fromEnvironment('MYBUILDS_CHANNEL', defaultValue: 'internal');
    expect(find.text(channel), findsOneWidget);
  });
}
