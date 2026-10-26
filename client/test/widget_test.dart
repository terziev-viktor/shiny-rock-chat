import 'package:flutter_test/flutter_test.dart';

import 'package:shiny_rock_chat/main.dart';

void main() {
  testWidgets('app shell starts', (WidgetTester tester) async {
    await tester.pumpWidget(const ShinyRockChatApp());

    expect(find.byType(ShinyRockChatApp), findsOneWidget);
  });
}
